package service

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/cachex"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/samber/hot"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func automaticAffinityContextForTest(userID int, path, body string) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	common.SetContextKey(c, constant.ContextKeyUserId, userID)
	return c
}

func TestAutomaticChannelAffinity(t *testing.T) {
	for _, useRedis := range []bool{false, true} {
		t.Run(fmt.Sprintf("redis=%t", useRedis), func(t *testing.T) {
			setting := operation_setting.GetChannelAffinitySetting()
			previousSetting, previousCache := *setting, getChannelAffinityCache()
			t.Cleanup(func() { *setting = previousSetting; channelAffinityCache = previousCache })
			*setting = operation_setting.ChannelAffinitySetting{Enabled: true, SessionMode: "prefer", DefaultTTLSeconds: 600, MaxEntries: 100}
			server := miniredis.RunT(t)
			client := redis.NewClient(&redis.Options{Addr: server.Addr()})
			t.Cleanup(func() { require.NoError(t, client.Close()) })
			channelAffinityCache = cachex.NewHybridCache[int](cachex.HybridCacheConfig[int]{
				Namespace: cachex.Namespace(channelAffinityCacheNamespace), Redis: client, RedisCodec: cachex.IntCodec{},
				RedisEnabled: func() bool { return useRedis },
				Memory:       func() *hot.HotCache[string, int] { return hot.NewHotCache[string, int](hot.LRU, 100).Build() },
			})
			const initial = `{"messages":[{"role":"system","content":"PRIVATE_SYSTEM"},{"role":"user","content":"PRIVATE_FIRST_QUESTION"}],"tools":[{"type":"function","function":{"name":"lookup"}}],"prompt_cache_options":{"ttl":"1h"}}`
			const continued = `{"tools":[{"function":{"name":"lookup"},"type":"function"}],"messages":[{"content":"PRIVATE_SYSTEM","role":"system"},{"content":"PRIVATE_FIRST_QUESTION","role":"user"},{"role":"assistant","content":"answer"},{"role":"user","content":"next question"}]}`
			seed := automaticAffinityContextForTest(42, "/v1/chat/completions", initial)
			_, found := GetPreferredChannelByAffinity(seed, "GLM-5.3", "default")
			require.False(t, found)
			meta, ok := getChannelAffinityMeta(seed)
			require.True(t, ok)
			assert.Equal(t, 3600, meta.TTLSeconds, "automatic bindings must cover 1h caches even with a 600s global TTL")
			assert.Equal(t, "prefer", RequestPolicy(seed).SessionMode)
			assert.NotContains(t, meta.CacheKey, "PRIVATE_")
			RecordChannelAffinity(seed, 101)
			followup := automaticAffinityContextForTest(42, "/v1/chat/completions", continued)
			channelID, found := GetPreferredChannelByAffinity(followup, "GLM-5.3", "default")
			require.True(t, found)
			assert.Equal(t, 101, channelID, "additional turns and object key ordering must keep the binding")
			RecordChannelAffinity(followup, 101)
			decision := DecideRelayRetry(followup, types.NewErrorWithStatusCode(errors.New("first chunk timeout"), types.ErrorCodeUpstreamFirstChunkTimeout, 499), 1)
			assert.Equal(t, "retry", decision.Action)
			for _, tc := range []struct {
				name, model, group, body string
				userID                   int
			}{
				{name: "another user", userID: 43, model: "GLM-5.3", group: "default", body: continued},
				{name: "another model", userID: 42, model: "kimi-3", group: "default", body: continued},
				{name: "another group", userID: 42, model: "GLM-5.3", group: "other", body: continued},
				{name: "changed system", userID: 42, model: "GLM-5.3", group: "default", body: strings.Replace(continued, "PRIVATE_SYSTEM", "changed", 1)},
				{name: "changed first question", userID: 42, model: "GLM-5.3", group: "default", body: strings.Replace(continued, "PRIVATE_FIRST_QUESTION", "new conversation", 1)},
				{name: "changed tools", userID: 42, model: "GLM-5.3", group: "default", body: strings.Replace(continued, "lookup", "another_tool", 1)},
			} {
				t.Run(tc.name, func(t *testing.T) {
					c := automaticAffinityContextForTest(tc.userID, "/v1/chat/completions", tc.body)
					_, hit := GetPreferredChannelByAffinity(c, tc.model, tc.group)
					assert.False(t, hit)
				})
			}
			if useRedis {
				assert.Equal(t, time.Hour, server.TTL(meta.CacheKey))
				server.FastForward(11 * time.Minute)
				c := automaticAffinityContextForTest(42, "/v1/chat/completions", continued)
				_, hit := GetPreferredChannelByAffinity(c, "GLM-5.3", "default")
				assert.True(t, hit, "binding survives beyond Magik's 600s default")
			}
			stats := GetChannelAffinityCacheStats()
			assert.Greater(t, stats.ByRuleName[automaticAffinityRuleName], 0)
			removed, err := ClearChannelAffinityCacheByRuleName(automaticAffinityRuleName)
			require.NoError(t, err)
			assert.Greater(t, removed, 0)
		})
	}
}

func TestAutomaticAffinityIdentityAndOptOut(t *testing.T) {
	setting := operation_setting.GetChannelAffinitySetting()
	previous := *setting
	t.Cleanup(func() { *setting = previous })
	*setting = operation_setting.ChannelAffinitySetting{Enabled: true, DefaultTTLSeconds: 3600, SessionMode: "prefer"}
	for _, tc := range []struct {
		name, path, body string
		supported        bool
	}{
		{name: "OpenAI Chat", path: "/v1/chat/completions", body: `{"messages":[{"role":"user","content":"hi"}]}`, supported: true},
		{name: "Claude Messages", path: "/v1/messages", body: `{"system":"system","messages":[{"role":"user","content":[{"type":"text","text":"hi"}]}]}`, supported: true},
		{name: "Responses string", path: "/v1/responses", body: `{"instructions":"system","input":"hi"}`, supported: true},
		{name: "Responses messages", path: "/v1/responses", body: `{"input":[{"role":"user","content":[{"type":"input_text","text":"hi"}]}]}`, supported: true},
		{name: "Gemini", path: "/v1beta/models/gemini:streamGenerateContent", body: `{"systemInstruction":{"parts":[{"text":"system"}]},"contents":[{"parts":[{"text":"hi"}]}]}`, supported: true},
		{name: "missing first user", path: "/v1/chat/completions", body: `{"messages":[{"role":"system","content":"system"}]}`},
		{name: "empty first user", path: "/v1/chat/completions", body: `{"messages":[{"role":"user","content":[]}]}`},
		{name: "image task excluded", path: "/v1/images/generations", body: `{"prompt_cache_key":"fixed","messages":[{"role":"user","content":"hi"}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := automaticAffinityContextForTest(42, tc.path, tc.body)
			_, _ = GetPreferredChannelByAffinity(c, "test-model", "default")
			_, ok := getChannelAffinityMeta(c)
			assert.Equal(t, tc.supported, ok)
		})
	}
	first := automaticAffinityContextForTest(42, "/v1/chat/completions", `{"prompt_cache_key":"body-key","messages":[{"role":"user","content":"first"}]}`)
	first.Request.Header.Set("X-Session-Id", "header-key")
	second := automaticAffinityContextForTest(42, "/v1/chat/completions", `{"prompt_cache_key":"different","messages":[{"role":"user","content":"changed"}]}`)
	second.Request.Header.Set("X-Session-Id", "header-key")
	assert.Equal(t, automaticChannelAffinityKey(first, "GLM-5.3", "default"), automaticChannelAffinityKey(second, "GLM-5.3", "default"), "explicit headers take precedence over body and generated identities")
	bodyFirst := automaticAffinityContextForTest(42, "/v1/chat/completions", `{"prompt_cache_key":"stable-body-key","messages":[{"role":"user","content":"one"}]}`)
	bodySecond := automaticAffinityContextForTest(42, "/v1/chat/completions", `{"prompt_cache_key":"stable-body-key","messages":[{"role":"user","content":"two"}]}`)
	assert.Equal(t, automaticChannelAffinityKey(bodyFirst, "GLM-5.3", "default"), automaticChannelAffinityKey(bodySecond, "GLM-5.3", "default"))
	assert.Empty(t, automaticChannelAffinityKey(automaticAffinityContextForTest(0, "/v1/chat/completions", `{"input":"hi"}`), "GLM-5.3", "default"))
	image1 := automaticAffinityContextForTest(42, "/v1/chat/completions", `{"messages":[{"role":"user","content":[{"type":"text","text":"describe"},{"type":"image_url","image_url":{"url":"https://example.com/a"}}]}]}`)
	image2 := automaticAffinityContextForTest(42, "/v1/chat/completions", `{"messages":[{"role":"user","content":[{"type":"text","text":"describe"},{"type":"image_url","image_url":{"url":"https://example.com/b"}}]}]}`)
	assert.NotEqual(t, automaticChannelAffinityKey(image1, "GLM-5.3", "default"), automaticChannelAffinityKey(image2, "GLM-5.3", "default"))
	for _, mode := range []string{"disabled", "global off", "rule off"} {
		t.Run(mode, func(t *testing.T) {
			*setting = operation_setting.ChannelAffinitySetting{Enabled: true, SessionMode: "prefer"}
			switch mode {
			case "disabled":
				setting.Enabled = false
			case "global off":
				setting.SessionMode = "off"
			case "rule off":
				setting.Rules = []operation_setting.ChannelAffinityRule{{Name: "disabled GLM", ModelRegex: []string{"^GLM"}, SessionMode: "off"}}
			}
			c := automaticAffinityContextForTest(42, "/v1/chat/completions", `{"messages":[{"role":"user","content":"hi"}]}`)
			_, _ = GetPreferredChannelByAffinity(c, "GLM-5.3", "default")
			RecordChannelAffinity(c, 999)
			_, found := GetPreferredChannelByAffinity(automaticAffinityContextForTest(42, "/v1/chat/completions", `{"messages":[{"role":"user","content":"hi"}]}`), "GLM-5.3", "default")
			assert.False(t, found)
		})
	}
}

func buildChannelAffinityTemplateContextForTest(meta channelAffinityMeta) *gin.Context {
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	setChannelAffinityContext(ctx, meta)
	return ctx
}

func TestApplyChannelAffinityOverrideTemplate_NoTemplate(t *testing.T) {
	ctx := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{
		RuleName: "rule-no-template",
	})
	base := map[string]any{
		"temperature": 0.7,
	}

	merged, applied := ApplyChannelAffinityOverrideTemplate(ctx, base)
	require.False(t, applied)
	require.Equal(t, base, merged)
}

func TestApplyChannelAffinityOverrideTemplate_MergeTemplate(t *testing.T) {
	ctx := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{
		RuleName: "rule-with-template",
		ParamTemplate: map[string]any{
			"temperature": 0.2,
			"top_p":       0.95,
		},
		UsingGroup:     "default",
		ModelName:      "gpt-4.1",
		RequestPath:    "/v1/responses",
		KeySourceType:  "gjson",
		KeySourcePath:  "prompt_cache_key",
		KeyHint:        "abcd...wxyz",
		KeyFingerprint: "abcd1234",
	})
	base := map[string]any{
		"temperature": 0.7,
		"max_tokens":  2000,
	}

	merged, applied := ApplyChannelAffinityOverrideTemplate(ctx, base)
	require.True(t, applied)
	require.Equal(t, 0.7, merged["temperature"])
	require.Equal(t, 0.95, merged["top_p"])
	require.Equal(t, 2000, merged["max_tokens"])
	require.Equal(t, 0.7, base["temperature"])

	anyInfo, ok := ctx.Get(ginKeyChannelAffinityLogInfo)
	require.True(t, ok)
	info, ok := anyInfo.(map[string]any)
	require.True(t, ok)
	overrideInfoAny, ok := info["override_template"]
	require.True(t, ok)
	overrideInfo, ok := overrideInfoAny.(map[string]any)
	require.True(t, ok)
	require.Equal(t, true, overrideInfo["applied"])
	require.Equal(t, "rule-with-template", overrideInfo["rule_name"])
	require.EqualValues(t, 2, overrideInfo["param_override_keys"])
}

func TestApplyChannelAffinityOverrideTemplate_MergeOperations(t *testing.T) {
	ctx := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{
		RuleName: "rule-with-ops-template",
		ParamTemplate: map[string]any{
			"operations": []map[string]any{
				{
					"mode":  "pass_headers",
					"value": []string{"Originator"},
				},
			},
		},
	})
	base := map[string]any{
		"temperature": 0.7,
		"operations": []map[string]any{
			{
				"path":  "model",
				"mode":  "trim_prefix",
				"value": "openai/",
			},
		},
	}

	merged, applied := ApplyChannelAffinityOverrideTemplate(ctx, base)
	require.True(t, applied)
	require.Equal(t, 0.7, merged["temperature"])

	opsAny, ok := merged["operations"]
	require.True(t, ok)
	ops, ok := opsAny.([]any)
	require.True(t, ok)
	require.Len(t, ops, 2)

	firstOp, ok := ops[0].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "pass_headers", firstOp["mode"])

	secondOp, ok := ops[1].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "trim_prefix", secondOp["mode"])
}

func TestShouldSkipRetryAfterChannelAffinityFailure(t *testing.T) {
	tests := []struct {
		name string
		ctx  func() *gin.Context
		want bool
	}{
		{
			name: "nil context",
			ctx: func() *gin.Context {
				return nil
			},
			want: false,
		},
		{
			name: "explicit skip retry flag in context",
			ctx: func() *gin.Context {
				ctx := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{
					RuleName:   "rule-explicit-flag",
					SkipRetry:  false,
					UsingGroup: "default",
					ModelName:  "gpt-5",
				})
				ctx.Set(ginKeyChannelAffinitySkipRetry, true)
				return ctx
			},
			want: true,
		},
		{
			name: "fallback to matched rule meta",
			ctx: func() *gin.Context {
				return buildChannelAffinityTemplateContextForTest(channelAffinityMeta{
					RuleName:   "rule-skip-retry",
					SkipRetry:  true,
					UsingGroup: "default",
					ModelName:  "gpt-5",
				})
			},
			want: true,
		},
		{
			name: "no flag and no skip retry meta",
			ctx: func() *gin.Context {
				return buildChannelAffinityTemplateContextForTest(channelAffinityMeta{
					RuleName:   "rule-no-skip-retry",
					SkipRetry:  false,
					UsingGroup: "default",
					ModelName:  "gpt-5",
				})
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, ShouldSkipRetryAfterChannelAffinityFailure(tt.ctx()))
		})
	}
}

func TestExtractChannelAffinityValue_RequestHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Request.Header.Set("X-Affinity-Key", " tenant-123 ")

	value := extractChannelAffinityValue(ctx, operation_setting.ChannelAffinityKeySource{
		Type: "request_header",
		Key:  "X-Affinity-Key",
	})

	require.Equal(t, "tenant-123", value)
}

func TestGetPreferredChannelByAffinity_RequestHeaderKeySource(t *testing.T) {
	gin.SetMode(gin.TestMode)

	rule := operation_setting.ChannelAffinityRule{
		Name:       "header-affinity",
		ModelRegex: []string{"^gpt-.*$"},
		PathRegex:  []string{"/v1/responses"},
		KeySources: []operation_setting.ChannelAffinityKeySource{
			{Type: "request_header", Key: "X-Affinity-Key"},
		},
		IncludeRuleName:  true,
		IncludeModelName: true,
	}

	affinityValue := fmt.Sprintf("header-hit-%d", time.Now().UnixNano())
	cacheKeySuffix := buildChannelAffinityCacheKeySuffix(rule, "gpt-5", "default", affinityValue)

	cache := getChannelAffinityCache()
	require.NoError(t, cache.SetWithTTL(cacheKeySuffix, 9528, time.Minute))
	t.Cleanup(func() {
		_, _ = cache.DeleteMany([]string{cacheKeySuffix})
	})

	setting := operation_setting.GetChannelAffinitySetting()
	originalRules := setting.Rules
	setting.Rules = append([]operation_setting.ChannelAffinityRule{rule}, originalRules...)
	t.Cleanup(func() {
		setting.Rules = originalRules
	})

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Request.Header.Set("X-Affinity-Key", affinityValue)
	ctx.Request.Header.Set("X-Session-Id", "automatic-fallback-must-not-win")
	common.SetContextKey(ctx, constant.ContextKeyUserId, 42)

	channelID, found := GetPreferredChannelByAffinity(ctx, "gpt-5", "default")
	require.True(t, found)
	require.Equal(t, 9528, channelID)

	meta, ok := getChannelAffinityMeta(ctx)
	require.True(t, ok)
	require.Equal(t, "request_header", meta.KeySourceType)
	require.Equal(t, "X-Affinity-Key", meta.KeySourceKey)
	require.Equal(t, buildChannelAffinityKeyHint(affinityValue), meta.KeyHint)
}

func TestClearCurrentChannelAffinityCache(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cacheKeySuffix := fmt.Sprintf("codex cli trace:default:clear-current-%d", time.Now().UnixNano())
	cacheKeyFull := channelAffinityCacheNamespace + ":" + cacheKeySuffix
	cache := getChannelAffinityCache()
	require.NoError(t, cache.SetWithTTL(cacheKeySuffix, 9527, time.Minute))
	t.Cleanup(func() {
		_, _ = cache.DeleteMany([]string{cacheKeySuffix})
	})

	ctx := buildChannelAffinityTemplateContextForTest(channelAffinityMeta{
		CacheKey:   cacheKeyFull,
		TTLSeconds: 60,
		RuleName:   "codex cli trace",
		SkipRetry:  true,
	})
	require.True(t, ShouldSkipRetryAfterChannelAffinityFailure(ctx))

	deleted := ClearCurrentChannelAffinityCache(ctx)
	require.True(t, deleted)
	_, found, err := cache.Get(cacheKeySuffix)
	require.NoError(t, err)
	require.False(t, found)
	require.False(t, ShouldSkipRetryAfterChannelAffinityFailure(ctx))
}

func TestChannelAffinityHitCodexTemplatePassHeadersEffective(t *testing.T) {
	gin.SetMode(gin.TestMode)

	setting := operation_setting.GetChannelAffinitySetting()
	require.NotNil(t, setting)

	var codexRule *operation_setting.ChannelAffinityRule
	for i := range setting.Rules {
		rule := &setting.Rules[i]
		if strings.EqualFold(strings.TrimSpace(rule.Name), "codex cli trace") {
			codexRule = rule
			break
		}
	}
	require.NotNil(t, codexRule)

	affinityValue := fmt.Sprintf("pc-hit-%d", time.Now().UnixNano())
	cacheKeySuffix := buildChannelAffinityCacheKeySuffix(*codexRule, "gpt-5", "default", affinityValue)

	cache := getChannelAffinityCache()
	require.NoError(t, cache.SetWithTTL(cacheKeySuffix, 9527, time.Minute))
	t.Cleanup(func() {
		_, _ = cache.DeleteMany([]string{cacheKeySuffix})
	})

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(fmt.Sprintf(`{"prompt_cache_key":"%s"}`, affinityValue)))
	ctx.Request.Header.Set("Content-Type", "application/json")

	channelID, found := GetPreferredChannelByAffinity(ctx, "gpt-5", "default")
	require.True(t, found)
	require.Equal(t, 9527, channelID)

	baseOverride := map[string]any{
		"temperature": 0.2,
	}
	mergedOverride, applied := ApplyChannelAffinityOverrideTemplate(ctx, baseOverride)
	require.True(t, applied)
	require.Equal(t, 0.2, mergedOverride["temperature"])

	info := &relaycommon.RelayInfo{
		RequestHeaders: map[string]string{
			"Originator": "Codex CLI",
			"Session_id": "sess-123",
			"User-Agent": "codex-cli-test",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ParamOverride: mergedOverride,
			HeadersOverride: map[string]any{
				"X-Static": "legacy-static",
			},
		},
	}

	_, err := relaycommon.ApplyParamOverrideWithRelayInfo([]byte(`{"model":"gpt-5"}`), info)
	require.NoError(t, err)
	require.True(t, info.UseRuntimeHeadersOverride)

	require.Equal(t, "legacy-static", info.RuntimeHeadersOverride["x-static"])
	require.Equal(t, "Codex CLI", info.RuntimeHeadersOverride["originator"])
	require.Equal(t, "sess-123", info.RuntimeHeadersOverride["session_id"])
	require.Equal(t, "codex-cli-test", info.RuntimeHeadersOverride["user-agent"])

	_, exists := info.RuntimeHeadersOverride["x-codex-beta-features"]
	require.False(t, exists)
	_, exists = info.RuntimeHeadersOverride["x-codex-turn-metadata"]
	require.False(t, exists)
}

func TestMidjourneyPolicyAcceptance(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		code       int
		properties map[string]any
		requestErr error
		accepted   bool
		reason     string
	}{
		{name: "submitted", status: 200, code: 1, accepted: true},
		{name: "existing task", status: 200, code: 21, accepted: true},
		{name: "queued", status: 200, code: 22, accepted: true},
		{name: "HTTP 200 with rejected prompt", status: 200, code: 24, reason: "non_retryable_error"},
		{name: "failed existing task", status: 200, code: 21, properties: map[string]any{"status": "FAILURE"}, reason: "task_accepted"},
		{name: "HTTP failure", status: 502, code: 1, reason: "non_retryable_error"},
		{name: "ambiguous transport failure", status: 502, requestErr: errors.New("connection reset"), reason: "non_retryable_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
			ctx.Request = httptest.NewRequest(http.MethodPost, "/mj/submit/imagine", strings.NewReader(`{}`))
			ctx.Set("channel_id", 7)
			state := RequestPolicy(ctx)
			state.BeginAttempt(&model.Channel{Id: 7}, "default")
			response := &dto.MidjourneyResponseWithStatusCode{StatusCode: tc.status, Response: dto.MidjourneyResponse{Code: tc.code, Properties: tc.properties}}
			accepted := RecordMidjourneyPolicyResponse(ctx, response, tc.requestErr)
			assert.Equal(t, tc.accepted, accepted)
			assert.False(t, state.Successful, "acceptance alone must not bind before local processing finishes")
			if accepted {
				MarkRequestPolicySuccess(ctx, nil)
				assert.True(t, state.Successful)
				return
			}
			events := state.Events()
			require.Len(t, events, 3)
			assert.Equal(t, PolicyDecision{Action: "failure", Reason: "upstream_failure", Source: "upstream"}, events[1].Decision)
			assert.Equal(t, tc.status, events[1].Status)
			assert.Equal(t, PolicyDecision{Action: "stop", Reason: tc.reason, Source: "system"}, events[2].Decision, "submissions are never replayed")
		})
	}
}

func TestSessionRulesInheritOrOverrideGlobalDefault(t *testing.T) {
	setting := operation_setting.GetChannelAffinitySetting()
	previous := *setting
	t.Cleanup(func() { *setting = previous })
	for _, globalMode := range []string{"", "off", "prefer", "strict"} {
		for _, tc := range []struct {
			name, mode, expected string
			skipRetry            bool
		}{
			{name: "inherit", mode: "inherit", expected: globalMode},
			{name: "override-strict", mode: "strict", expected: "strict"},
			{name: "override-prefer", mode: "prefer", expected: "prefer", skipRetry: true},
			{name: "override-off", mode: "off", expected: "off", skipRetry: true},
			{name: "legacy-strict", expected: "strict", skipRetry: true},
			{name: "legacy-prefer", expected: "prefer"},
		} {
			t.Run(globalMode+"/"+tc.name, func(t *testing.T) {
				expected := tc.expected
				if expected == "" {
					expected = "prefer"
				}
				source := "session_rule"
				if tc.mode == "inherit" {
					source = "global"
				}
				rule := operation_setting.ChannelAffinityRule{
					Name: tc.name, ModelRegex: []string{"^test-model$"},
					KeySources:  []operation_setting.ChannelAffinityKeySource{{Type: "request_header", Key: "X-Session"}},
					SessionMode: tc.mode, SkipRetryOnFailure: tc.skipRetry,
					ParamOverrideTemplate: map[string]any{"temperature": 0}, IncludeRuleName: true,
				}
				encoded, err := common.Marshal([]operation_setting.ChannelAffinityRule{rule})
				require.NoError(t, err)
				snapshot, err := model.BuildRequestPolicy(map[string]string{"RetryTimes": "2", "channel_affinity_setting.enabled": "true", "channel_affinity_setting.session_mode": globalMode, "channel_affinity_setting.rules": string(encoded)})
				require.NoError(t, err)
				*setting = snapshot.Affinity
				key := t.Name()
				cacheKey := buildChannelAffinityCacheKeySuffix(rule, "test-model", "default", key)
				cache := getChannelAffinityCache()
				require.NoError(t, cache.SetWithTTL(cacheKey, 1, time.Minute))
				t.Cleanup(func() { _, err := cache.DeleteMany([]string{cacheKey}); assert.NoError(t, err) })
				ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
				ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
				ctx.Request.Header.Set("X-Session", key)
				state := RequestPolicy(ctx)
				channelID, found := GetPreferredChannelByAffinity(ctx, "test-model", "default")
				assert.Equal(t, expected != "off", found)
				if found {
					assert.Equal(t, 1, channelID)
				}
				assert.Equal(t, expected, state.SessionMode)
				assert.Equal(t, source, state.SessionModeSource)
				assert.Equal(t, expected == "strict", ShouldSkipRetryAfterChannelAffinityFailure(ctx))
				decision := DecideRelayRetry(ctx, types.NewOpenAIError(errors.New("upstream"), types.ErrorCodeBadResponseStatusCode, http.StatusTooManyRequests), 2)
				if expected == "strict" {
					assert.Equal(t, PolicyDecision{Action: "stop", Reason: "strict_session", Source: source}, decision)
				} else {
					assert.Equal(t, "retry", decision.Action)
				}
				events := state.Events()
				require.Len(t, events, 1)
				assert.Equal(t, PolicyDecision{Action: "match", Reason: "session_rule_matched", Source: "session_rule"}, events[0].Decision)
				assert.Equal(t, tc.name, events[0].Rule)
				transformed, applied := ApplyChannelAffinityOverrideTemplate(ctx, nil)
				assert.True(t, applied, "session behavior does not disable request transforms")
				assert.EqualValues(t, 0, transformed["temperature"])
				if expected == "off" {
					ctx.Set("channel_id", 9)
					RecordChannelAffinity(ctx, 9)
					cached, found, err := cache.Get(cacheKey)
					require.NoError(t, err)
					assert.True(t, found)
					assert.Equal(t, 1, cached, "off does not refresh session bindings")
					_, err = cache.DeleteMany([]string{cacheKey})
					require.NoError(t, err)
					RecordChannelAffinity(ctx, 9)
					_, found, err = cache.Get(cacheKey)
					require.NoError(t, err)
					assert.False(t, found, "off does not establish session bindings")
				}
			})
		}
	}
}
