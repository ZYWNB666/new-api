// Package gatewaylimit enforces cached, layered gateway admission limits.
package gatewaylimit

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"unicode"
)

const OptionKey = "GatewayRateLimits"
const PolicyTopic = "newapi:gateway-limits:policy"
const MaxRules = 2000
const MaxValue = 1000000000

type Rule struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Scope      string `json:"scope"`
	SubjectID  int    `json:"subject_id"`
	Model      string `json:"model"`
	ChannelIDs []int  `json:"channel_ids"`
	RPM        int64  `json:"rpm"`
	TPM        int64  `json:"tpm"`
	Concurrent int64  `json:"concurrent"`
	Enabled    bool   `json:"enabled"`
}

type Config struct {
	Revision int64  `json:"revision"`
	Rules    []Rule `json:"rules"`
}

type Policy struct {
	Config      Config
	ProviderTPM bool
	byID        map[string]Rule
	users       map[int][]Rule
	keys        map[int][]Rule
	models      map[int]map[string][]Rule
	providers   map[int][]Rule
}

var current atomic.Pointer[Policy]

// Initialize is only called when initializing the database option cache.
func Initialize() { current.Store(emptyPolicy) }

func Current() *Policy {
	if p := current.Load(); p != nil {
		return p
	}
	return emptyPolicy
}

var emptyPolicy = &Policy{Config: Config{Rules: []Rule{}}}

// Publish ignores delayed option refreshes and pub/sub messages from older revisions.
func Publish(p *Policy) {
	for {
		old := current.Load()
		if old != nil && old.Config.Revision > p.Config.Revision {
			return
		}
		if current.CompareAndSwap(old, p) {
			return
		}
	}
}

func Compile(config Config) (*Policy, error) {
	if config.Revision < 0 || config.Revision > 1<<52 || len(config.Rules) > MaxRules {
		return nil, fmt.Errorf("invalid policy revision or rule count")
	}
	p := &Policy{Config: Config{Revision: config.Revision, Rules: slices.Clone(config.Rules)}, users: map[int][]Rule{}, keys: map[int][]Rule{}, models: map[int]map[string][]Rule{}, providers: map[int][]Rule{}, byID: map[string]Rule{}}
	if p.Config.Rules == nil {
		p.Config.Rules = []Rule{}
	}
	ids := map[string]bool{}
	targets := map[string]bool{}
	for i, r := range p.Config.Rules {
		if r.ID == "" || len(r.ID) > 64 || ids[r.ID] {
			return nil, fmt.Errorf("invalid or duplicate rule ID")
		}
		for _, ch := range r.ID {
			if !unicode.IsLetter(ch) && !unicode.IsDigit(ch) && ch != '-' && ch != '_' {
				return nil, fmt.Errorf("invalid rule ID")
			}
		}
		ids[r.ID] = true
		r.Name = strings.TrimSpace(r.Name)
		r.Model = strings.TrimSpace(r.Model)
		if r.Name == "" || len(r.Name) > 128 || len(r.Model) > 256 {
			return nil, fmt.Errorf("invalid rule name or model")
		}
		if r.RPM < 0 || r.TPM < 0 || r.Concurrent < 0 || r.RPM > MaxValue || r.TPM > MaxValue || r.Concurrent > MaxValue {
			return nil, fmt.Errorf("limits must be between 0 and %d", MaxValue)
		}
		if r.RPM == 0 && r.TPM == 0 && r.Concurrent == 0 {
			return nil, fmt.Errorf("at least one limit must be positive")
		}
		r.ChannelIDs = slices.Clone(r.ChannelIDs)
		slices.Sort(r.ChannelIDs)
		r.ChannelIDs = slices.Compact(r.ChannelIDs)
		if r.ChannelIDs == nil {
			r.ChannelIDs = []int{}
		}
		switch r.Scope {
		case "user", "api_key", "user_model":
			if r.SubjectID <= 0 || len(r.ChannelIDs) != 0 || (r.Scope == "user_model" && r.Model == "") || (r.Scope != "user_model" && r.Model != "") {
				return nil, fmt.Errorf("invalid rule target")
			}
			target := r.Scope + ":" + strconv.Itoa(r.SubjectID) + ":" + r.Model
			if targets[target] {
				return nil, fmt.Errorf("duplicate rule target")
			}
			targets[target] = true
		case "provider":
			if r.SubjectID != 0 || r.Model != "" || len(r.ChannelIDs) == 0 || len(r.ChannelIDs) > 1000 || r.ChannelIDs[0] <= 0 {
				return nil, fmt.Errorf("invalid provider channel pool")
			}
		default:
			return nil, fmt.Errorf("invalid rule scope")
		}
		p.Config.Rules[i] = r
		p.byID[r.ID] = r
		if !r.Enabled {
			continue
		}
		switch r.Scope {
		case "user":
			p.users[r.SubjectID] = append(p.users[r.SubjectID], r)
		case "api_key":
			p.keys[r.SubjectID] = append(p.keys[r.SubjectID], r)
		case "user_model":
			if p.models[r.SubjectID] == nil {
				p.models[r.SubjectID] = map[string][]Rule{}
			}
			p.models[r.SubjectID][r.Model] = append(p.models[r.SubjectID][r.Model], r)
		case "provider":
			p.ProviderTPM = p.ProviderTPM || r.TPM > 0
			for _, id := range r.ChannelIDs {
				p.providers[id] = append(p.providers[id], r)
				if len(p.providers[id]) > 8 {
					return nil, fmt.Errorf("a channel may match at most 8 provider limits")
				}
			}
		}
	}
	return p, nil
}

func (p *Policy) RequestRules(userID, keyID int, model string) []Rule {
	rules := append([]Rule(nil), p.users[userID]...)
	rules = append(rules, p.keys[keyID]...)
	return append(rules, p.models[userID][model]...)
}
func (p *Policy) ProviderRules(channelID int) []Rule { return p.providers[channelID] }
func (p *Policy) Enabled() bool                      { return len(p.users)+len(p.keys)+len(p.models)+len(p.providers) > 0 }

func (p *Policy) Rule(id string) (Rule, bool) { rule, found := p.byID[id]; return rule, found }
