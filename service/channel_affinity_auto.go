package service

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const automaticAffinityRuleName = "automatic session"

// Configured rules retain precedence. The built-in fallback inherits the global
// session mode, but retains bindings for at least the one-hour cache window.
func channelAffinityRules(setting *operation_setting.ChannelAffinitySetting) []operation_setting.ChannelAffinityRule {
	rules := slices.Clone(setting.Rules)
	return append(rules, operation_setting.ChannelAffinityRule{
		Name: automaticAffinityRuleName, ModelRegex: []string{".+"},
		PathRegex:   []string{`^/(v1|pg)/(chat/completions|completions|responses|messages)$`, `^/v1(beta)?/models/[^/]+:(generateContent|streamGenerateContent)$`},
		KeySources:  []operation_setting.ChannelAffinityKeySource{{Type: "automatic"}},
		SessionMode: "inherit", TTLSeconds: max(3600, setting.DefaultTTLSeconds),
		IncludeUsingGroup: true, IncludeModelName: true, IncludeRuleName: true,
	})
}

// automaticChannelAffinityKey is a routing fingerprint, never a provider cache
// parameter. It hashes canonical JSON, so object ordering and concatenation
// ambiguity do not affect the key, and no prompt/session text enters the cache.
// The boolean reports whether prompt content, rather than an explicit ID, was used.
func automaticChannelAffinityKey(c *gin.Context, modelName, usingGroup string) (string, bool) {
	if c == nil || c.Request == nil || c.Request.Method != http.MethodPost {
		return "", false
	}
	userID := common.GetContextKeyInt(c, constant.ContextKeyUserId)
	if userID <= 0 {
		return "", false
	}
	var identity any
	fromPrompt := false
	for _, name := range []string{"X-Session-Id", "X-Conversation-Id", "Session-Id", "Session_id", "Thread-Id", "Thread_id"} {
		if value := strings.TrimSpace(c.GetHeader(name)); value != "" {
			identity = []any{"header", name, value}
			break
		}
	}
	if identity == nil {
		storage, err := common.GetBodyStorage(c)
		if err != nil {
			return "", false
		}
		body, err := storage.Bytes()
		if err != nil || !gjson.ValidBytes(body) {
			return "", false
		}
		root := gjson.ParseBytes(body)
		for _, path := range []string{"prompt_cache_key", "metadata.session_id", "metadata.conversation_id", "session_id", "conversation.id", "conversation"} {
			value := root.Get(path)
			if value.Type == gjson.String && strings.TrimSpace(value.String()) != "" {
				identity = []any{"body", path, strings.TrimSpace(value.String())}
				break
			}
		}
		if identity == nil {
			var systems []any
			for _, name := range []string{"system", "instructions", "systemInstruction"} {
				if value := root.Get(name); value.Exists() {
					systems = append(systems, value.Value())
				}
			}
			var firstUser gjson.Result
			if input := root.Get("input"); input.Type == gjson.String {
				firstUser = input
			}
			for _, name := range []string{"messages", "input", "contents"} {
				for _, message := range root.Get(name).Array() {
					role := message.Get("role").String()
					content := message.Get("content")
					if name == "contents" {
						content = message.Get("parts")
					}
					if role == "system" || role == "developer" {
						systems = append(systems, content.Value())
					}
					if !firstUser.Exists() && (role == "user" || name == "contents" && role == "") {
						firstUser = content
					}
				}
			}
			if !firstUser.Exists() || firstUser.Type == gjson.Null || firstUser.Type == gjson.String && strings.TrimSpace(firstUser.String()) == "" || firstUser.IsArray() && len(firstUser.Array()) == 0 {
				return "", false
			}
			fromPrompt = true
			identity = []any{"content", c.Request.URL.Path, systems, root.Get("tools").Value(), firstUser.Value()}
		}
	}
	data, err := common.Marshal([]any{"automatic-affinity-v1", userID, modelName, usingGroup, identity})
	if err != nil {
		return "", false
	}
	sum := sha256.Sum256(data)
	return "session-" + hex.EncodeToString(sum[:]), fromPrompt
}
