package gatewaylimit

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"math"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/go-redis/redis/v8"
	"github.com/thanhpk/randstr"
)

//go:embed limit.lua
var scriptSource string
var script = redis.NewScript(scriptSource)

const maxTokens int64 = 1 << 42

type Rejection struct {
	Rule       Rule
	Dimension  string
	RetryAfter int
}

func (r *Rejection) Error() string {
	return fmt.Sprintf("gateway %s %s limit exceeded", r.Rule.Scope, r.Dimension)
}
func (r *Rejection) HTTPStatusCode() int { return 429 }

var ErrUnavailable = errors.New("gateway rate limit storage unavailable")

type Usage struct {
	ID         string `json:"id"`
	RPM        int64  `json:"rpm"`
	TPM        int64  `json:"tpm"`
	Concurrent int64  `json:"concurrent"`
}

type bucket struct {
	reserved int64
	rpm, tpm float64
	updated  time.Time
	active   map[string]int64
}

// Manager keeps no request history: token buckets plus only active reservations.
// Redis acquisitions/finishes each use one Lua call for ALL matching limits.
type Manager struct {
	backend   func() (*redis.Client, bool)
	now       func() time.Time
	mu        sync.Mutex
	buckets   map[string]*bucket
	active    map[string]*Lease
	heartbeat sync.Once
	stop      chan struct{}
}

type Lease struct {
	manager  *Manager
	id       string
	rules    []Rule
	estimate int64
	actual   atomic.Int64
	sent     atomic.Bool
	once     sync.Once
	redis    bool
}

func NewManager(backend func() (*redis.Client, bool)) *Manager {
	return &Manager{backend: backend, now: time.Now, buckets: map[string]*bucket{}, active: map[string]*Lease{}, stop: make(chan struct{})}
}

var Default = NewManager(func() (*redis.Client, bool) { return common.RDB, common.RedisEnabled })

func keysFor(rules []Rule) []string {
	keys := make([]string, 0, len(rules)*2)
	for _, r := range rules {
		key := "gateway-limit:{gateway}:" + r.ID
		keys = append(keys, key, key+":active")
	}
	return keys
}
func arguments(operation, id string, cost int64, rules []Rule) []any {
	args := []any{operation, id, cost}
	for _, r := range rules {
		args = append(args, r.RPM, r.TPM, r.Concurrent)
	}
	return args
}
func (m *Manager) eval(ctx context.Context, operation, id string, cost int64, rules []Rule) ([]int64, error) {
	client, required := m.backend()
	if !required || client == nil {
		return nil, ErrUnavailable
	}
	bounded, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	result, err := script.Run(bounded, client, keysFor(rules), arguments(operation, id, cost, rules)...).Slice()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	values := make([]int64, len(result))
	for i, v := range result {
		switch n := v.(type) {
		case int64:
			values[i] = n
		case string:
			values[i], err = strconv.ParseInt(n, 10, 64)
		default:
			err = fmt.Errorf("unexpected limiter reply")
		}
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
	}
	return values, nil
}

func (m *Manager) Acquire(ctx context.Context, rules []Rule, estimate int64) (*Lease, error) {
	if len(rules) == 0 {
		return nil, nil
	}
	if estimate < 0 || estimate > maxTokens {
		return nil, fmt.Errorf("invalid token estimate")
	}
	l := &Lease{manager: m, id: randstr.String(24), rules: rules, estimate: estimate}
	l.actual.Store(-1)
	_, l.redis = m.backend()
	if l.redis {
		values, err := m.eval(ctx, "take", l.id, estimate, rules)
		if err != nil {
			// A timeout may occur after Redis committed. Release that known ID if possible.
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
			defer cancel()
			_, _ = m.eval(cleanup, "finish", l.id, 0, rules)
			return nil, err
		}
		if len(values) == 0 {
			return nil, ErrUnavailable
		}
		if values[0] == 0 {
			if len(values) != 4 || values[1] < 1 || values[1] > int64(len(rules)) || values[2] < 1 || values[2] > 3 {
				return nil, ErrUnavailable
			}
			return nil, &Rejection{Rule: rules[values[1]-1], Dimension: []string{"", "rpm", "tpm", "concurrent"}[values[2]], RetryAfter: int(values[3])}
		}
		m.mu.Lock()
		m.active[l.id] = l
		m.mu.Unlock()
		m.heartbeat.Do(func() { go m.renewLeases() })
	} else {
		m.mu.Lock()
		defer m.mu.Unlock()
		now := m.now()
		for _, r := range rules {
			b := m.localBucket(r, now)
			reserved := b.reserved
			if r.RPM > 0 && b.rpm+1 > float64(r.RPM)+1e-9 {
				return nil, &Rejection{Rule: r, Dimension: "rpm", RetryAfter: max(1, int(math.Ceil((b.rpm+1-float64(r.RPM))/float64(r.RPM)*60)))}
			}
			if r.TPM > 0 && b.tpm+float64(reserved+estimate) > float64(r.TPM)+1e-9 {
				return nil, &Rejection{Rule: r, Dimension: "tpm", RetryAfter: max(1, min(86400, int(math.Ceil((b.tpm+float64(reserved+estimate)-float64(r.TPM))/float64(r.TPM)*60))))}
			}
			if r.Concurrent > 0 && int64(len(b.active)) >= r.Concurrent {
				return nil, &Rejection{Rule: r, Dimension: "concurrent", RetryAfter: 1}
			}
		}
		for _, r := range rules {
			b := m.localBucket(r, now)
			if r.RPM > 0 {
				b.rpm++
			}
			amount := int64(0)
			if r.TPM > 0 {
				amount = estimate
			}
			b.active[l.id] = amount
			b.reserved += amount
		}
	}
	return l, nil
}
func (m *Manager) localBucket(r Rule, now time.Time) *bucket {
	b := m.buckets[r.ID]
	if b == nil {
		b = &bucket{updated: now, active: map[string]int64{}}
		m.buckets[r.ID] = b
	}
	elapsed := max(0, now.Sub(b.updated).Seconds())
	b.rpm = max(0, b.rpm-elapsed*float64(r.RPM)/60)
	b.tpm = max(0, b.tpm-elapsed*float64(r.TPM)/60)
	if r.RPM == 0 {
		b.rpm = 0
	}
	if r.TPM == 0 {
		b.tpm = 0
	}
	b.updated = now
	return b
}
func (l *Lease) MarkSent() {
	if l != nil {
		l.sent.Store(true)
	}
}
func (l *Lease) ReportUsage(tokens int64) {
	if l != nil && tokens >= 0 && tokens <= maxTokens {
		l.actual.Store(tokens)
	}
}
func (l *Lease) Finish() {
	if l == nil {
		return
	}
	l.once.Do(func() {
		actual := l.actual.Load()
		if actual < 0 {
			actual = 0
			if l.sent.Load() {
				actual = l.estimate
			}
		}
		m := l.manager
		// Completion uses current rates, so an old lease cannot erase or rapidly refill a newly tightened limit.
		rules := l.rules
		ratesChanged := false
		for i, rule := range l.rules {
			if updated, found := Current().Rule(rule.ID); found && (updated.RPM != rule.RPM || updated.TPM != rule.TPM || updated.Concurrent != rule.Concurrent) {
				if !ratesChanged {
					rules = append([]Rule(nil), l.rules...)
					ratesChanged = true
				}
				rules[i] = updated
			}
		}
		if l.redis {
			m.mu.Lock()
			delete(m.active, l.id)
			m.mu.Unlock()
			if _, err := m.eval(context.Background(), "finish", l.id, actual, rules); err != nil {
				logger.LogError(context.Background(), "gateway limit release failed: "+err.Error())
			}
			return
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		now := m.now()
		for _, r := range rules {
			b := m.localBucket(r, now)
			if reserved, ok := b.active[l.id]; ok {
				b.reserved -= reserved
				delete(b.active, l.id)
				if r.TPM > 0 {
					b.tpm += float64(actual)
				}
			}
		}
		// Bound idle local state when administrators replace rules repeatedly.
		if len(m.buckets) > MaxRules*2 {
			for id, b := range m.buckets {
				if len(b.active) == 0 && now.Sub(b.updated) > time.Hour {
					delete(m.buckets, id)
				}
			}
		}
	})
}
func (m *Manager) Inspect(ctx context.Context, rules []Rule) ([]Usage, error) {
	if len(rules) == 0 {
		return []Usage{}, nil
	}
	result := make([]Usage, len(rules))
	_, required := m.backend()
	if required {
		values, err := m.eval(ctx, "inspect", "", 0, rules)
		if err != nil {
			return nil, err
		}
		if len(values) != len(rules)*3 {
			return nil, ErrUnavailable
		}
		for i, r := range rules {
			result[i] = Usage{ID: r.ID, RPM: values[i*3], TPM: values[i*3+1], Concurrent: values[i*3+2]}
		}
	} else {
		m.mu.Lock()
		defer m.mu.Unlock()
		now := m.now()
		for i, r := range rules {
			b := m.localBucket(r, now)
			reserved := b.reserved
			result[i] = Usage{ID: r.ID, RPM: int64(math.Ceil(b.rpm)), TPM: int64(math.Ceil(b.tpm)) + reserved, Concurrent: int64(len(b.active))}
		}
	}
	return result, nil
}
func (m *Manager) renewLeases() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-m.stop:
			return
		case <-ticker.C:
		}
		m.mu.Lock()
		leases := make([]*Lease, 0, len(m.active))
		for _, l := range m.active {
			leases = append(leases, l)
		}
		m.mu.Unlock()
		// Batches bound Lua execution time and request size while sharing network RTTs.
		for start := 0; start < len(leases); start += 256 {
			batch := leases[start:min(start+256, len(leases))]
			var keys []string
			args := []any{"renew"}
			for _, l := range batch {
				keys = append(keys, keysFor(l.rules)...)
				args = append(args, l.id, len(l.rules))
			}
			client, required := m.backend()
			if !required || client == nil {
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			err := script.Run(ctx, client, keys, args...).Err()
			cancel()
			if err != nil {
				logger.LogError(context.Background(), "gateway limit lease renewal failed: "+err.Error())
			}
		}
	}
}
func (m *Manager) Close() { close(m.stop) }
