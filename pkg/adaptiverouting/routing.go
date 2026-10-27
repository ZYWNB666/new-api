// Package adaptiverouting keeps bounded, process-local channel/model health samples.
// It never changes configured channel weights or relaxes caller eligibility checks.
package adaptiverouting

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	observationTTL    = 10 * time.Minute
	maxObservedRoutes = 10000
)

type Sample struct {
	ChannelID    int
	Model        string
	Outcome      string
	Status       int
	RetryAfter   string
	TTFT         time.Duration
	Generation   time.Duration
	OutputTokens int64
}

type Candidate struct {
	ID     int
	Weight float64
}

type key struct {
	channel int
	model   string
}
type health struct {
	updated               time.Time
	samples               int
	success               float64
	ttft, tokens, seconds float64
	failures              int
	cooldown, retryUntil  time.Time
}

type Store struct {
	mu      sync.Mutex
	entries map[key]*health
}

func NewStore() *Store { return &Store{entries: make(map[key]*health)} }

// Default shares observations among all users on this gateway process. Samples
// expire after ten minutes; neither credentials nor request content are retained.
var Default = NewStore()

// RateLimitedError means every otherwise eligible candidate is backing off an
// upstream 429. Slow scores alone can never produce this error.
type RateLimitedError struct{ Until time.Time }

func (e *RateLimitedError) Error() string {
	return "all eligible upstreams are temporarily rate limited"
}
func (e *RateLimitedError) RetryAfter(now time.Time) string {
	return strconv.FormatInt(max(1, int64(math.Ceil(e.Until.Sub(now).Seconds()))), 10)
}

func (s *Store) Record(sample Sample, now time.Time) {
	if sample.ChannelID <= 0 || sample.Model == "" || len(sample.Model) > 256 || sample.Outcome == "ignored" {
		return
	}
	// Caller errors must not poison provider health. Explicit stream failures have
	// their protocol status supplied by the caller, even when HTTP already was 200.
	switch sample.Status {
	case 400, 405, 409, 413, 415, 422:
		return
	}
	if sample.Outcome != "success" && sample.Outcome != "failure" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key{sample.ChannelID, sample.Model}
	h := s.entries[k]
	if h == nil || (now.Sub(h.updated) > observationTTL && !h.retryUntil.After(now)) {
		if len(s.entries) >= maxObservedRoutes {
			for oldKey, old := range s.entries {
				if now.Sub(old.updated) > observationTTL && !old.retryUntil.After(now) {
					delete(s.entries, oldKey)
				}
			}
			if len(s.entries) >= maxObservedRoutes {
				return
			}
		}
		h = &health{success: 1}
		s.entries[k] = h
	}
	h.updated = now
	h.samples = min(h.samples+1, 100)
	success := sample.Outcome == "success"
	value := 0.0
	if success {
		value = 1
	}
	h.success = .8*h.success + .2*value
	if !success {
		h.failures = min(h.failures+1, 8)
		if sample.Status == http.StatusTooManyRequests || h.failures >= 3 {
			h.cooldown = now.Add(time.Duration(min(60, 5*(1<<min(h.failures-1, 4)))) * time.Second)
		}
		if sample.Status == http.StatusTooManyRequests {
			raw := strings.TrimSpace(sample.RetryAfter)
			until, err := http.ParseTime(raw)
			if seconds, parseErr := strconv.ParseUint(raw, 10, 32); parseErr == nil {
				until, err = now.Add(time.Duration(seconds)*time.Second), nil
			}
			// A 429 without a usable header still needs exponential backoff. Unlike
			// a poor performance score or a 5xx cooldown, this is real capacity loss.
			if err != nil || !until.After(now) {
				until = h.cooldown
			}
			if until.After(h.retryUntil) {
				h.retryUntil = until
			}
		}
		return
	}
	h.failures = 0
	h.cooldown = time.Time{}
	if sample.TTFT > 0 {
		value := sample.TTFT.Seconds()
		if h.ttft == 0 {
			h.ttft = value
		} else {
			h.ttft = .8*h.ttft + .2*value
		}
	}
	// EWMA quantities and durations together, rather than averaging per-request
	// TPS. Unknown/estimated usage and hidden reasoning are excluded by the caller.
	if sample.OutputTokens > 1 && sample.Generation > 0 {
		tokens, seconds := float64(sample.OutputTokens-1), sample.Generation.Seconds()
		if h.seconds == 0 {
			h.tokens, h.seconds = tokens, seconds
		} else {
			h.tokens, h.seconds = .8*h.tokens+.2*tokens, .8*h.seconds+.2*seconds
		}
	}
}

// Weights applies performance and reliability only within the caller's already
// filtered priority tier. A 5% exploration share permits recovery and cold starts.
func (s *Store) Weights(model string, candidates []Candidate, now time.Time) ([]float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	weights := make([]float64, len(candidates))
	active := make([]bool, len(candidates))
	healthy := false
	var earliest time.Time
	for i, candidate := range candidates {
		if candidate.Weight < 0 || math.IsNaN(candidate.Weight) || math.IsInf(candidate.Weight, 0) {
			return nil, fmt.Errorf("invalid routing weight")
		}
		if candidate.Weight == 0 {
			continue
		}
		h := s.entries[key{candidate.ID, model}]
		if h != nil && h.retryUntil.After(now) {
			if earliest.IsZero() || h.retryUntil.Before(earliest) {
				earliest = h.retryUntil
			}
			continue
		}
		active[i] = true
		if h == nil || !h.cooldown.After(now) {
			healthy = true
		}
	}
	// Compare only eligible peers in this priority tier. A baseline needs ten
	// recent samples so one cold-start response cannot redefine the whole pool.
	bestTTFT, bestTPS := 0.0, 0.0
	for i, candidate := range candidates {
		if !active[i] {
			continue
		}
		h := s.entries[key{candidate.ID, model}]
		if healthy && h != nil && h.cooldown.After(now) {
			active[i] = false
			continue
		}
		if h == nil || now.Sub(h.updated) > observationTTL || h.samples < 10 {
			continue
		}
		if h.ttft > 0 && (bestTTFT == 0 || h.ttft < bestTTFT) {
			bestTTFT = h.ttft
		}
		if h.seconds > 0 && h.tokens > 0 {
			bestTPS = max(bestTPS, h.tokens/h.seconds)
		}
	}
	baseSum, scoreSum := 0.0, 0.0
	for i, candidate := range candidates {
		if !active[i] {
			continue
		}
		h := s.entries[key{candidate.ID, model}]
		base := candidate.Weight
		// The caller preserves zero-weight and all-zero smoothing semantics.
		baseSum += base
		factor := 1.0
		if h != nil && now.Sub(h.updated) <= observationTTL {
			ttft, tps := 1.0, 1.0
			// Normalize against the best observed TTFT and TPS separately. Missing
			// measurements remain neutral; there are no absolute performance targets.
			if h.ttft > 0 && bestTTFT > 0 {
				ttft = min(1, bestTTFT/h.ttft)
			}
			if h.seconds > 0 && h.tokens > 0 && bestTPS > 0 {
				tps = min(1, h.tokens/h.seconds/bestTPS)
			}
			confidence := min(1, float64(h.samples)/10)
			performance := math.Pow(ttft, .6) * math.Pow(tps, .4)
			performance = 1 - confidence + confidence*performance
			factor = performance * performance * math.Pow(h.success, 3)
		}
		weights[i] = base * factor
		scoreSum += weights[i]
	}
	if baseSum == 0 {
		// If all positive-weight routes are cooling, use them as the soft fallback.
		// Upstream 429 backoff remains a hard boundary.
		anyActive := false
		for _, value := range active {
			anyActive = anyActive || value
		}
		if !anyActive && !earliest.IsZero() {
			return nil, &RateLimitedError{Until: earliest}
		}
		return weights, nil
	}
	for i, candidate := range candidates {
		if !active[i] {
			continue
		}
		if scoreSum <= 0 {
			weights[i] = candidate.Weight
			continue
		}
		weights[i] = .95*weights[i]/scoreSum + .05*candidate.Weight/baseSum
	}
	return weights, nil
}
