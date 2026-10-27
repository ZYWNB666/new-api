package adaptiverouting

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPerformanceAndReliabilityAdjustOnlyTheRequestedModel(t *testing.T) {
	now := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	candidates := []Candidate{{ID: 1, Weight: 100}, {ID: 2, Weight: 100}}
	for _, tc := range []struct {
		name    string
		ttft    time.Duration
		tokens  int64
		failure bool
	}{
		{name: "slow first token", ttft: 8 * time.Second, tokens: 41},
		{name: "slow generation", ttft: time.Second, tokens: 6},
		{name: "fast but fails", ttft: time.Millisecond, tokens: 100, failure: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := NewStore()
			for range 10 {
				outcome := "success"
				if tc.failure {
					outcome = "failure"
				}
				store.Record(Sample{ChannelID: 1, Model: "glm-5.3", Outcome: outcome, TTFT: tc.ttft, Generation: time.Second, OutputTokens: tc.tokens}, now)
				store.Record(Sample{ChannelID: 2, Model: "glm-5.3", Outcome: "success", TTFT: time.Second, Generation: time.Second, OutputTokens: 41}, now)
			}
			weights, err := store.Weights("glm-5.3", candidates, now.Add(time.Minute))
			require.NoError(t, err)
			assert.Less(t, weights[0], weights[1])
			assert.GreaterOrEqual(t, weights[0], .025, "retain exploration traffic")
			weights, err = store.Weights("other-model", candidates, now)
			require.NoError(t, err)
			assert.Equal(t, []float64{.5, .5}, weights)
		})
	}
}

func TestAllDegradedCandidatesStillRouteAndStaleSamplesRecover(t *testing.T) {
	now := time.Now()
	store := NewStore()
	candidates := []Candidate{{ID: 1, Weight: 1}, {ID: 2, Weight: 3}}
	for _, c := range candidates {
		for range 3 {
			store.Record(Sample{ChannelID: c.ID, Model: "glm", Outcome: "failure", Status: 503}, now)
		}
	}
	weights, err := store.Weights("glm", candidates, now)
	require.NoError(t, err)
	assert.Greater(t, weights[0], 0.0)
	assert.Greater(t, weights[1], 0.0)
	weights, err = store.Weights("glm", candidates, now.Add(11*time.Minute))
	require.NoError(t, err)
	assert.InDeltaSlice(t, []float64{.25, .75}, weights, 1e-12)
}

func TestRateLimitHonorsRetryAfterAndUsesAnAlternative(t *testing.T) {
	now := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	for _, header := range []string{"30", now.Add(30 * time.Second).Format(http.TimeFormat)} {
		t.Run(header, func(t *testing.T) {
			store := NewStore()
			store.Record(Sample{ChannelID: 1, Model: "glm", Outcome: "failure", Status: 429, RetryAfter: header}, now)
			weights, err := store.Weights("glm", []Candidate{{ID: 1, Weight: 1}, {ID: 2, Weight: 1}}, now)
			require.NoError(t, err)
			assert.Equal(t, []float64{0, 1}, weights)
			_, err = store.Weights("glm", []Candidate{{ID: 1, Weight: 1}}, now)
			var limited *RateLimitedError
			require.ErrorAs(t, err, &limited)
			assert.Equal(t, "30", limited.RetryAfter(now))
			weights, err = store.Weights("glm", []Candidate{{ID: 1, Weight: 1}}, now.Add(31*time.Second))
			require.NoError(t, err)
			assert.Equal(t, []float64{1}, weights)
		})
	}
}

func TestCallerErrorsAndMissingThroughputDoNotPenalizeHealth(t *testing.T) {
	now := time.Now()
	store := NewStore()
	for _, sample := range []Sample{
		{ChannelID: 1, Model: "glm", Outcome: "ignored", Status: 200},
		{ChannelID: 1, Model: "glm", Outcome: "failure", Status: 400},
		{ChannelID: 1, Model: "glm", Outcome: "success", Status: 200, TTFT: time.Second},
	} {
		store.Record(sample, now)
	}
	weights, err := store.Weights("glm", []Candidate{{ID: 1, Weight: 1}, {ID: 2, Weight: 1}}, now)
	require.NoError(t, err)
	assert.Equal(t, []float64{.5, .5}, weights)
}

func TestZeroWeightDoesNotPreventCooldownFallback(t *testing.T) {
	now := time.Now()
	store := NewStore()
	for range 3 {
		store.Record(Sample{ChannelID: 1, Model: "glm", Outcome: "failure", Status: 503}, now)
	}
	weights, err := store.Weights("glm", []Candidate{{ID: 1, Weight: 1}, {ID: 2, Weight: 0}}, now)
	require.NoError(t, err)
	assert.Equal(t, []float64{1, 0}, weights)
}

func TestRateLimitWithoutRetryAfterBacksOffInsteadOfHammeringUpstream(t *testing.T) {
	now := time.Now()
	store := NewStore()
	candidates := []Candidate{{ID: 1, Weight: 1}}
	for _, expected := range []string{"5", "10"} {
		store.Record(Sample{ChannelID: 1, Model: "glm", Outcome: "failure", Status: 429, RetryAfter: "invalid"}, now)
		_, err := store.Weights("glm", candidates, now)
		var limited *RateLimitedError
		require.ErrorAs(t, err, &limited)
		assert.Equal(t, expected, limited.RetryAfter(now))
	}
	weights, err := store.Weights("glm", candidates, now.Add(11*time.Second))
	require.NoError(t, err)
	assert.Equal(t, []float64{1}, weights)
}

func TestRelativePerformanceRanksPeersWithoutAbsoluteTargets(t *testing.T) {
	now := time.Now()
	candidates := []Candidate{{ID: 1, Weight: 1}, {ID: 2, Weight: 1}}
	var reference []float64
	for _, scale := range []int{1, 100} {
		t.Run(fmt.Sprintf("scale=%d", scale), func(t *testing.T) {
			store := NewStore()
			for range 10 {
				store.Record(Sample{ChannelID: 1, Model: "glm", Outcome: "success", TTFT: time.Duration(scale) * 100 * time.Millisecond, Generation: time.Duration(scale) * time.Second, OutputTokens: 201}, now)
				store.Record(Sample{ChannelID: 2, Model: "glm", Outcome: "success", TTFT: time.Duration(scale) * 500 * time.Millisecond, Generation: time.Duration(scale) * time.Second, OutputTokens: 101}, now)
			}
			weights, err := store.Weights("glm", candidates, now)
			require.NoError(t, err)
			assert.Greater(t, weights[0], weights[1], "prefer the better peer even when all channels are fast or all are slow")
			assert.Greater(t, weights[1], 0.0, "slower peers retain exploration traffic")
			if reference == nil {
				reference = weights
			} else {
				assert.InDeltaSlice(t, reference, weights, 1e-12, "absolute performance scale must not change relative allocation")
			}
		})
	}
}
