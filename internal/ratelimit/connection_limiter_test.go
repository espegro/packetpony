package ratelimit

import (
	"sync"
	"testing"
)

func TestConnectionLimiter_Allow(t *testing.T) {
	limiter := NewConnectionLimiter(3)

	ip := "192.168.1.1"

	// First 3 concurrent connections should be allowed
	for i := 0; i < 3; i++ {
		if !limiter.Allow(ip) {
			t.Errorf("Connection %d should be allowed", i+1)
		}
	}

	// 4th concurrent connection should be denied
	if limiter.Allow(ip) {
		t.Error("4th connection should be denied")
	}

	// Release one connection
	limiter.Release(ip)

	// Now one more should be allowed
	if !limiter.Allow(ip) {
		t.Error("Connection after release should be allowed")
	}
}

// TestConnectionLimiter_LongLivedRelease verifies the core fix for Alt A:
// releasing a connection frees exactly one slot regardless of ordering, so a
// long-lived connection that closes does not corrupt the count of others.
func TestConnectionLimiter_LongLivedRelease(t *testing.T) {
	limiter := NewConnectionLimiter(2)

	ip := "192.168.1.1"

	// Open two concurrent connections (at the limit).
	if !limiter.Allow(ip) || !limiter.Allow(ip) {
		t.Fatal("first two connections should be allowed")
	}
	if limiter.Allow(ip) {
		t.Fatal("third concurrent connection should be denied")
	}

	// The first (long-lived) connection closes; exactly one slot frees up.
	limiter.Release(ip)
	if !limiter.Allow(ip) {
		t.Error("a slot should be free after one release")
	}
	if limiter.Allow(ip) {
		t.Error("only one slot should have been freed")
	}
}

func TestConnectionLimiter_MultipleIPs(t *testing.T) {
	limiter := NewConnectionLimiter(2)

	ip1 := "192.168.1.1"
	ip2 := "192.168.1.2"

	// Each IP should have independent quota
	if !limiter.Allow(ip1) {
		t.Error("IP1 first connection should be allowed")
	}
	if !limiter.Allow(ip1) {
		t.Error("IP1 second connection should be allowed")
	}
	if limiter.Allow(ip1) {
		t.Error("IP1 third connection should be denied")
	}

	// IP2 should still have quota
	if !limiter.Allow(ip2) {
		t.Error("IP2 first connection should be allowed")
	}
	if !limiter.Allow(ip2) {
		t.Error("IP2 second connection should be allowed")
	}
	if limiter.Allow(ip2) {
		t.Error("IP2 third connection should be denied")
	}
}

func TestConnectionLimiter_Release(t *testing.T) {
	limiter := NewConnectionLimiter(2)

	ip := "192.168.1.1"

	// Use up quota
	limiter.Allow(ip)
	limiter.Allow(ip)

	// Release both
	limiter.Release(ip)
	limiter.Release(ip)

	// Should be able to allocate again
	if !limiter.Allow(ip) {
		t.Error("Connection after releases should be allowed")
	}
	if !limiter.Allow(ip) {
		t.Error("Second connection after releases should be allowed")
	}
}

func TestConnectionLimiter_ReleaseNonExistent(t *testing.T) {
	limiter := NewConnectionLimiter(2)

	// Releasing non-existent IP should not panic
	limiter.Release("192.168.1.1")
}

// TestConnectionLimiter_EntryRemovedAtZero verifies the map self-prunes so it
// does not grow unbounded across many short-lived clients.
func TestConnectionLimiter_EntryRemovedAtZero(t *testing.T) {
	limiter := NewConnectionLimiter(5)

	ip := "192.168.1.1"
	limiter.Allow(ip)
	limiter.Release(ip)

	limiter.mu.Lock()
	_, exists := limiter.active[ip]
	limiter.mu.Unlock()
	if exists {
		t.Error("entry should be removed once its count reaches zero")
	}
}

func TestConnectionLimiter_ConcurrentAccess(t *testing.T) {
	limiter := NewConnectionLimiter(100)

	const numGoroutines = 50
	ip := "192.168.1.1"

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Each goroutine acquires and releases a slot; with balanced Allow/Release
	// the final count must return to zero and the entry must be pruned.
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			if limiter.Allow(ip) {
				limiter.Release(ip)
			}
		}()
	}

	wg.Wait()

	limiter.mu.Lock()
	count := limiter.active[ip]
	limiter.mu.Unlock()
	if count != 0 {
		t.Errorf("expected balanced acquire/release to leave count 0, got %d", count)
	}
}

func TestConnectionLimiter_ZeroLimit(t *testing.T) {
	limiter := NewConnectionLimiter(0)

	ip := "192.168.1.1"

	// With 0 limit, all connections should be denied
	if limiter.Allow(ip) {
		t.Error("Connection should be denied with 0 limit")
	}
}

func BenchmarkConnectionLimiter_Allow(b *testing.B) {
	limiter := NewConnectionLimiter(1000)

	ip := "192.168.1.1"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Allow(ip)
	}
}

func BenchmarkConnectionLimiter_AllowMultipleIPs(b *testing.B) {
	limiter := NewConnectionLimiter(100)

	ips := []string{
		"192.168.1.1",
		"192.168.1.2",
		"192.168.1.3",
		"192.168.1.4",
		"192.168.1.5",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ip := ips[i%len(ips)]
		limiter.Allow(ip)
	}
}

func BenchmarkConnectionLimiter_ConcurrentAllow(b *testing.B) {
	limiter := NewConnectionLimiter(1000)

	b.RunParallel(func(pb *testing.PB) {
		ip := "192.168.1.1"
		for pb.Next() {
			limiter.Allow(ip)
		}
	})
}
