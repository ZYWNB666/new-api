package gatewaylimit

import (
	"context"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLayeredAdmission(t *testing.T) {
	for _, backend := range []string{"memory", "redis"} {
		t.Run(backend, func(t *testing.T) {
			var client *redis.Client
			if backend == "redis" {
				address := os.Getenv("TEST_GATEWAY_REDIS_ADDR")
				if address == "" {
					t.Skip("TEST_GATEWAY_REDIS_ADDR is not configured")
				}
				host, _, err := net.SplitHostPort(address)
				require.NoError(t, err)
				require.True(t, net.ParseIP(host).IsLoopback(), "Redis integration tests require a dedicated loopback instance")
				client = redis.NewClient(&redis.Options{Addr: address, DB: 15})
				require.NoError(t, client.Ping(context.Background()).Err())
				// Dedicated test database, never the application's configured Redis.
				require.NoError(t, client.FlushDB(context.Background()).Err())
				t.Cleanup(func() { client.FlushDB(context.Background()); client.Close() })
			}
			m := NewManager(func() (*redis.Client, bool) { return client, client != nil })
			t.Cleanup(m.Close)
			ctx := context.Background()
			rules := []Rule{{ID: "user", RPM: 100, TPM: 100, Concurrent: 2}, {ID: "key", RPM: 100, TPM: 100, Concurrent: 1}}
			first, err := m.Acquire(ctx, rules, 20)
			require.NoError(t, err)
			first.MarkSent()
			_, err = m.Acquire(ctx, rules, 20)
			var rejection *Rejection
			require.ErrorAs(t, err, &rejection)
			assert.Equal(t, "concurrent", rejection.Dimension)
			assert.Equal(t, "key", rejection.Rule.ID)
			usage, err := m.Inspect(ctx, rules)
			require.NoError(t, err)
			assert.Equal(t, int64(1), usage[0].RPM, "a failed key check must not consume user RPM")
			assert.Equal(t, int64(20), usage[0].TPM)
			first.ReportUsage(70)
			first.Finish()
			first.Finish()
			usage, err = m.Inspect(ctx, rules)
			require.NoError(t, err)
			assert.Equal(t, int64(0), usage[0].Concurrent)
			assert.Equal(t, int64(70), usage[0].TPM)
			second, err := m.Acquire(ctx, rules, 20)
			require.NoError(t, err)
			second.ReportUsage(150)
			second.Finish()
			_, err = m.Acquire(ctx, rules, 1)
			require.ErrorAs(t, err, &rejection)
			assert.Equal(t, "tpm", rejection.Dimension)
			usage, err = m.Inspect(ctx, rules)
			require.NoError(t, err)
			assert.Greater(t, usage[0].TPM, int64(100), "actual output can temporarily exceed input admission capacity")
			// Parallel callers across managers share a supplier's capacity in Redis.
			pool := []Rule{{ID: "supplier", Concurrent: 3}}
			var wg sync.WaitGroup
			leases := make(chan *Lease, 8)
			other := m
			if client != nil {
				other = NewManager(func() (*redis.Client, bool) { return client, true })
				t.Cleanup(other.Close)
			}
			for attempt := range 8 {
				manager := m
				if attempt%2 == 1 {
					manager = other
				}
				wg.Go(func() {
					lease, err := manager.Acquire(ctx, pool, 0)
					if err == nil {
						leases <- lease
					} else {
						var reject *Rejection
						assert.ErrorAs(t, err, &reject)
					}
				})
			}
			wg.Wait()
			close(leases)
			assert.Len(t, leases, 3)
			for lease := range leases {
				lease.Finish()
			}
			usage, err = m.Inspect(ctx, pool)
			require.NoError(t, err)
			assert.Zero(t, usage[0].Concurrent)
			// A locally rejected request releases input reservations without adding usage.
			aborted, err := m.Acquire(ctx, pool, 0)
			require.NoError(t, err)
			aborted.Finish()
			if client != nil {
				crashRule := []Rule{{ID: "crash", Concurrent: 1, TPM: 100}}
				stale, err := m.Acquire(ctx, crashRule, 40)
				require.NoError(t, err)
				// Simulate expiry at the Redis clock boundary without sleeping.
				require.NoError(t, client.ZAdd(ctx, keysFor(crashRule)[1], &redis.Z{Score: 0, Member: stale.id}).Err())
				fresh, err := m.Acquire(ctx, crashRule, 40)
				require.NoError(t, err)
				stale.MarkSent()
				stale.Finish()
				usage, err := m.Inspect(ctx, crashRule)
				require.NoError(t, err)
				assert.Equal(t, int64(1), usage[0].Concurrent, "a late finish cannot release a replacement request")
				assert.Equal(t, int64(40), usage[0].TPM)
				fresh.Finish()
			}
		})
	}
}

func TestTPMCorrectionKeepsOtherInFlightInputs(t *testing.T) {
	m := NewManager(func() (*redis.Client, bool) { return nil, false })
	defer m.Close()
	now := time.Unix(100, 0)
	m.now = func() time.Time { return now }
	rules := []Rule{{ID: "a", TPM: 100, RPM: 2}}
	first, err := m.Acquire(context.Background(), rules, 80)
	require.NoError(t, err)
	now = now.Add(time.Minute)
	_, err = m.Acquire(context.Background(), rules, 30)
	require.Error(t, err, "input stays reserved while a long stream runs")
	second, err := m.Acquire(context.Background(), rules, 20)
	require.NoError(t, err)
	first.ReportUsage(0)
	first.Finish()
	usage, err := m.Inspect(context.Background(), rules)
	require.NoError(t, err)
	assert.Equal(t, int64(20), usage[0].TPM)
	second.MarkSent()
	second.Finish()
	usage, err = m.Inspect(context.Background(), rules)
	require.NoError(t, err)
	assert.Equal(t, int64(20), usage[0].TPM)
}

func TestPolicyTargetsAndUnavailableStorage(t *testing.T) {
	config := Config{Rules: []Rule{
		{ID: "u", Name: "User", Scope: "user", SubjectID: 1, RPM: 10, Enabled: true},
		{ID: "k", Name: "Key", Scope: "api_key", SubjectID: 2, TPM: 100, Enabled: true},
		{ID: "m", Name: "Model", Scope: "user_model", SubjectID: 1, Model: "model-a", Concurrent: 1, Enabled: true},
		{ID: "p", Name: "Supplier", Scope: "provider", ChannelIDs: []int{3, 4}, RPM: 10, Enabled: true},
	}}
	p, err := Compile(config)
	require.NoError(t, err)
	assert.Len(t, p.RequestRules(1, 2, "model-a"), 3)
	assert.Len(t, p.RequestRules(1, 2, "mapped-name"), 2)
	assert.Equal(t, p.ProviderRules(3), p.ProviderRules(4))
	config.Rules = append(config.Rules, Rule{ID: "duplicate", Name: "Duplicate", Scope: "user", SubjectID: 1, RPM: 1})
	_, err = Compile(config)
	require.Error(t, err)
	m := NewManager(func() (*redis.Client, bool) { return nil, true })
	defer m.Close()
	_, err = m.Acquire(context.Background(), p.RequestRules(1, 2, "model-a"), 1)
	require.ErrorIs(t, err, ErrUnavailable)
	lease, err := m.Acquire(context.Background(), nil, 0)
	require.NoError(t, err)
	assert.Nil(t, lease, "unmatched requests must not contact Redis")
}

func BenchmarkAdmission(b *testing.B) {
	for _, backend := range []string{"disabled", "memory", "redis"} {
		b.Run(backend, func(b *testing.B) {
			var client *redis.Client
			if backend == "redis" {
				address := os.Getenv("TEST_GATEWAY_REDIS_ADDR")
				if address == "" {
					b.Skip("TEST_GATEWAY_REDIS_ADDR not configured")
				}
				client = redis.NewClient(&redis.Options{Addr: address, DB: 15})
				defer client.Close()
			}
			m := NewManager(func() (*redis.Client, bool) { return client, client != nil })
			defer m.Close()
			rules := []Rule{{ID: "bench-user", RPM: MaxValue, TPM: MaxValue, Concurrent: 100}, {ID: "bench-key", RPM: MaxValue, TPM: MaxValue, Concurrent: 100}, {ID: "bench-model", RPM: MaxValue, TPM: MaxValue, Concurrent: 100}}
			if backend == "disabled" {
				rules = nil
			}
			ctx := context.Background()
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				lease, err := m.Acquire(ctx, rules, 100)
				if err != nil {
					b.Fatal(err)
				}
				lease.MarkSent()
				lease.Finish()
			}
		})
	}
}

func TestFinishingOldRequestPreservesNewLimit(t *testing.T) {
	Initialize()
	t.Cleanup(Initialize)
	m := NewManager(func() (*redis.Client, bool) { return nil, false })
	defer m.Close()
	now := time.Unix(100, 0)
	m.now = func() time.Time { return now }
	rule := Rule{ID: "updated", Name: "Updated", Scope: "user", SubjectID: 1, Concurrent: 2, Enabled: true}
	old, err := m.Acquire(context.Background(), []Rule{rule}, 0)
	require.NoError(t, err)
	rule.RPM = 1
	rule.TPM = 100
	p, err := Compile(Config{Revision: 1, Rules: []Rule{rule}})
	require.NoError(t, err)
	Publish(p)
	recent, err := m.Acquire(context.Background(), []Rule{rule}, 10)
	require.NoError(t, err)
	recent.ReportUsage(10)
	recent.Finish()
	old.Finish()
	_, err = m.Acquire(context.Background(), []Rule{rule}, 0)
	var rejection *Rejection
	require.ErrorAs(t, err, &rejection)
	assert.Equal(t, "rpm", rejection.Dimension, "an older unlimited lease cannot clear the new RPM counter")
}
