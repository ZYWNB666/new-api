package dto

import (
	"errors"
	"regexp"
	"strings"
	"sync"
)

var requestIDHeaderRules = struct {
	sync.RWMutex
	compiled map[string]*regexp.Regexp
}{compiled: make(map[string]*regexp.Regexp)}

// CompileRequestIDHeaderRule accepts an exact HTTP header name or re:RE2.
// Matching covers the entire name and ignores case by default. Values are
// never matched or rewritten. The shared cache is bounded across all tenants.
func CompileRequestIDHeaderRule(rule string) (*regexp.Regexp, error) {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return nil, nil
	}
	if len(rule) > 256 {
		return nil, errors.New("request ID header rule exceeds 256 bytes")
	}
	requestIDHeaderRules.RLock()
	cached := requestIDHeaderRules.compiled[rule]
	requestIDHeaderRules.RUnlock()
	if cached != nil {
		return cached, nil
	}
	pattern, isRegex := strings.CutPrefix(rule, "re:")
	if isRegex {
		if pattern == "" {
			return nil, errors.New("request ID header regex is empty")
		}
	} else {
		for _, r := range rule {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("!#$%&'*+-.^_`|~", r) {
				continue
			}
			return nil, errors.New("invalid request ID header name")
		}
		pattern = regexp.QuoteMeta(rule)
	}
	compiled, err := regexp.Compile("(?i)^(?:" + pattern + ")$")
	if err != nil {
		return nil, errors.New("invalid request ID header regex")
	}
	requestIDHeaderRules.Lock()
	if len(requestIDHeaderRules.compiled) < 1024 {
		requestIDHeaderRules.compiled[rule] = compiled
	}
	requestIDHeaderRules.Unlock()
	return compiled, nil
}
