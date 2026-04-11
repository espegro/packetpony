package ratelimit

import (
	"sync"
	"testing"
	"time"
)

func TestConnectionLimiter_Allow(t *testing.T) {
	limiter := NewConnectionLimiter(3, 1*time.Second)
	defer limiter.Close()

	ip := "192.168.1.1"

	// First 3 connections should be allowed
	for i := 0; i < 3; i++ {
		if !limiter.Allow(ip) {
			t.Errorf("Connection %d should be allowed", i+1)
		}
	}

	// 4th connection should be denied
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

func TestConnectionLimiter_SlidingWindow(t *testing.T) {
	window := 100 * time.Millisecond
	limiter := NewConnectionLimiter(2, window)
	defer limiter.Close()

	ip := "192.168.1.1"

	// Use up the quota
	if !limiter.Allow(ip) {
		t.Fatal("First connection should be allowed")
	}
	if !limiter.Allow(ip) {
		t.Fatal("Second connection should be allowed")
	}
	if limiter.Allow(ip) {
		t.Error("Third connection should be denied")
	}

	// Wait for window to expire
	time.Sleep(window + 50*time.Millisecond)

	// Should be allowed again
	if !limiter.Allow(ip) {
		t.Error("Connection after window expiry should be allowed")
	}
}

func TestConnectionLimiter_MultipleIPs(t *testing.T) {
	limiter := NewConnectionLimiter(2, 1*time.Second)
	defer limiter.Close()

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
	limiter := NewConnectionLimiter(2, 1*time.Second)
	defer limiter.Close()

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
	limiter := NewConnectionLimiter(2, 1*time.Second)
	defer limiter.Close()

	// Releasing non-existent IP should not panic
	limiter.Release("192.168.1.1")
}

func TestConnectionLimiter_ConcurrentAccess(t *testing.T) {
	limiter := NewConnectionLimiter(100, 1*time.Second)
	defer limiter.Close()

	const numGoroutines = 50
	const numOps = 10
	ip := "192.168.1.1"

	var wg sync.WaitGroup
	wg.Add(numGoroutines * 2)

	// Concurrent Allow operations
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOps; j++ {
				limiter.Allow(ip)
				time.Sleep(1 * time.Millisecond)
			}
		}()
	}

	// Concurrent Release operations
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < numOps; j++ {
				limiter.Release(ip)
				time.Sleep(1 * time.Millisecond)
			}
		}()
	}

	wg.Wait()

	// No panics = success
}

func TestConnectionLimiter_Cleanup(t *testing.T) {
	window := 50 * time.Millisecond
	limiter := NewConnectionLimiter(5, window)
	defer limiter.Close()

	ip := "192.168.1.1"

	// Create an entry
	limiter.Allow(ip)
	limiter.Release(ip)

	// Check that entry exists
	limiter.mu.RLock()
	_, exists := limiter.connections[ip]
	limiter.mu.RUnlock()
	if !exists {
		t.Fatal("Entry should exist")
	}

	// Wait for cleanup (window * 2 + margin)
	time.Sleep(window*2 + 100*time.Millisecond)

	// Entry should be cleaned up
	limiter.mu.RLock()
	_, exists = limiter.connections[ip]
	limiter.mu.RUnlock()
	if exists {
		t.Error("Entry should be cleaned up")
	}
}

func TestConnectionLimiter_ZeroLimit(t *testing.T) {
	limiter := NewConnectionLimiter(0, 1*time.Second)
	defer limiter.Close()

	ip := "192.168.1.1"

	// With 0 limit, all connections should be denied
	if limiter.Allow(ip) {
		t.Error("Connection should be denied with 0 limit")
	}
}

func BenchmarkConnectionLimiter_Allow(b *testing.B) {
	limiter := NewConnectionLimiter(1000, 1*time.Minute)
	defer limiter.Close()

	ip := "192.168.1.1"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Allow(ip)
	}
}

func BenchmarkConnectionLimiter_AllowMultipleIPs(b *testing.B) {
	limiter := NewConnectionLimiter(100, 1*time.Minute)
	defer limiter.Close()

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
	limiter := NewConnectionLimiter(1000, 1*time.Minute)
	defer limiter.Close()

	b.RunParallel(func(pb *testing.PB) {
		ip := "192.168.1.1"
		for pb.Next() {
			limiter.Allow(ip)
		}
	})
}
