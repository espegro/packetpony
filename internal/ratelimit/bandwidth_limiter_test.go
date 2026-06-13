package ratelimit

import (
	"sync"
	"testing"
	"time"
)

func TestBandwidthLimiter_Allow_DropMode(t *testing.T) {
	maxBytes := int64(1000)
	limiter := NewBandwidthLimiter(maxBytes, 1*time.Second, "drop", 0)
	defer limiter.Close()

	ip := "192.168.1.1"

	// Should allow up to the limit
	if !limiter.Allow(ip, 500) {
		t.Error("First 500 bytes should be allowed")
	}
	if !limiter.Allow(ip, 400) {
		t.Error("Next 400 bytes should be allowed (total 900)")
	}

	// Should deny when exceeding limit
	if limiter.Allow(ip, 200) {
		t.Error("Next 200 bytes should be denied (would exceed 1000)")
	}

	// Should still allow small amount that fits
	if !limiter.Allow(ip, 50) {
		t.Error("50 bytes should be allowed (total 950)")
	}
}

func TestBandwidthLimiter_AllowWithStatus(t *testing.T) {
	maxBytes := int64(1000)

	// log_only: traffic under the limit must NOT be reported as over limit.
	// This guards against the old double-counting bug where recording the
	// bytes and then re-checking the window counted the same bytes twice
	// (e.g. usage 0 + 600, then 600 + 600 > 1000 → false positive).
	limiter := NewBandwidthLimiter(maxBytes, 1*time.Second, "log_only", 0)
	defer limiter.Close()

	ip := "192.168.1.1"

	allowed, overLimit := limiter.AllowWithStatus(ip, 600)
	if !allowed {
		t.Error("600 bytes should be allowed in log_only mode")
	}
	if overLimit {
		t.Error("600 bytes against a 1000 limit must not be reported as over limit")
	}

	// Now push past the limit: 600 + 600 = 1200 > 1000.
	allowed, overLimit = limiter.AllowWithStatus(ip, 600)
	if !allowed {
		t.Error("log_only mode must still allow traffic over the limit")
	}
	if !overLimit {
		t.Error("second 600 bytes should be reported as over limit (total 1200)")
	}

	// drop mode: over-limit bytes are denied and reported as over limit,
	// but rejected bytes must not be recorded against the window.
	dropLimiter := NewBandwidthLimiter(maxBytes, 1*time.Second, "drop", 0)
	defer dropLimiter.Close()

	if allowed, overLimit := dropLimiter.AllowWithStatus(ip, 900); !allowed || overLimit {
		t.Errorf("900/1000 should be allowed and within limit, got allowed=%v overLimit=%v", allowed, overLimit)
	}
	if allowed, overLimit := dropLimiter.AllowWithStatus(ip, 200); allowed || !overLimit {
		t.Errorf("200 bytes (would be 1100) should be denied and over limit, got allowed=%v overLimit=%v", allowed, overLimit)
	}
	// The denied 200 bytes were not recorded, so 100 more still fits (900+100).
	if allowed, overLimit := dropLimiter.AllowWithStatus(ip, 100); !allowed || overLimit {
		t.Errorf("100 bytes (total 1000) should be allowed and within limit, got allowed=%v overLimit=%v", allowed, overLimit)
	}
}

func TestBandwidthLimiter_Allow_ThrottleMode(t *testing.T) {
	maxBytes := int64(1000)
	throttleMin := int64(100)
	limiter := NewBandwidthLimiter(maxBytes, 1*time.Second, "throttle", throttleMin)
	defer limiter.Close()

	ip := "192.168.1.1"

	// Use up the quota
	if !limiter.Allow(ip, 900) {
		t.Error("900 bytes should be allowed")
	}

	// Over limit, but small packet should be allowed (throttle mode)
	if !limiter.Allow(ip, 50) {
		t.Error("50 bytes should be allowed in throttle mode (< minimum)")
	}

	// Large packet should be denied even in throttle mode
	if limiter.Allow(ip, 200) {
		t.Error("200 bytes should be denied in throttle mode (> minimum)")
	}

	// Packet equal to minimum should be allowed
	if !limiter.Allow(ip, 100) {
		t.Error("100 bytes (= throttle minimum) should be allowed")
	}
}

func TestBandwidthLimiter_Allow_LogOnlyMode(t *testing.T) {
	maxBytes := int64(1000)
	limiter := NewBandwidthLimiter(maxBytes, 1*time.Second, "log_only", 0)
	defer limiter.Close()

	ip := "192.168.1.1"

	// All should be allowed in log_only mode
	if !limiter.Allow(ip, 500) {
		t.Error("500 bytes should be allowed")
	}
	if !limiter.Allow(ip, 600) {
		t.Error("600 bytes should be allowed (even over limit)")
	}
	if !limiter.Allow(ip, 1000) {
		t.Error("1000 bytes should be allowed (even way over limit)")
	}
}

func TestBandwidthLimiter_IsOverLimit(t *testing.T) {
	maxBytes := int64(1000)
	limiter := NewBandwidthLimiter(maxBytes, 1*time.Second, "drop", 0)
	defer limiter.Close()

	ip := "192.168.1.1"

	// Not over limit initially
	if limiter.IsOverLimit(ip, 500) {
		t.Error("500 bytes should not be over limit")
	}

	// Use some bandwidth
	limiter.Allow(ip, 800)

	// Now should be over limit
	if !limiter.IsOverLimit(ip, 300) {
		t.Error("Adding 300 bytes should be over limit (800+300 > 1000)")
	}

	// Small amount should not be over limit
	if limiter.IsOverLimit(ip, 100) {
		t.Error("Adding 100 bytes should not be over limit (800+100 <= 1000)")
	}
}

func TestBandwidthLimiter_SlidingWindow(t *testing.T) {
	window := 100 * time.Millisecond
	maxBytes := int64(1000)
	limiter := NewBandwidthLimiter(maxBytes, window, "drop", 0)
	defer limiter.Close()

	ip := "192.168.1.1"

	// Use up the quota
	if !limiter.Allow(ip, 1000) {
		t.Fatal("1000 bytes should be allowed")
	}

	// Should be denied now
	if limiter.Allow(ip, 100) {
		t.Error("Should be denied when quota exhausted")
	}

	// Wait for window to expire
	time.Sleep(window + 50*time.Millisecond)

	// Should be allowed again
	if !limiter.Allow(ip, 1000) {
		t.Error("Should be allowed after window expiry")
	}
}

func TestBandwidthLimiter_MultipleIPs(t *testing.T) {
	maxBytes := int64(1000)
	limiter := NewBandwidthLimiter(maxBytes, 1*time.Second, "drop", 0)
	defer limiter.Close()

	ip1 := "192.168.1.1"
	ip2 := "192.168.1.2"

	// Each IP should have independent quota
	if !limiter.Allow(ip1, 1000) {
		t.Error("IP1 should be allowed 1000 bytes")
	}
	if limiter.Allow(ip1, 100) {
		t.Error("IP1 should be denied more bytes")
	}

	// IP2 should still have full quota
	if !limiter.Allow(ip2, 1000) {
		t.Error("IP2 should be allowed 1000 bytes")
	}
	if limiter.Allow(ip2, 100) {
		t.Error("IP2 should be denied more bytes")
	}
}

func TestBandwidthLimiter_ZeroBytes(t *testing.T) {
	maxBytes := int64(1000)
	limiter := NewBandwidthLimiter(maxBytes, 1*time.Second, "drop", 0)
	defer limiter.Close()

	ip := "192.168.1.1"

	// Zero bytes should always be allowed
	if !limiter.Allow(ip, 0) {
		t.Error("Zero bytes should be allowed")
	}

	// Use up quota
	limiter.Allow(ip, 1000)

	// Zero bytes should still be allowed
	if !limiter.Allow(ip, 0) {
		t.Error("Zero bytes should be allowed even when quota exhausted")
	}
}

func TestBandwidthLimiter_ConcurrentAccess(t *testing.T) {
	maxBytes := int64(10000)
	limiter := NewBandwidthLimiter(maxBytes, 1*time.Second, "drop", 0)
	defer limiter.Close()

	const numGoroutines = 50
	const bytesPerOp = 10
	ip := "192.168.1.1"

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				limiter.Allow(ip, bytesPerOp)
				time.Sleep(1 * time.Millisecond)
			}
		}()
	}

	wg.Wait()

	// No panics = success
}

func TestBandwidthLimiter_DefaultAction(t *testing.T) {
	// Empty string should default to "drop"
	limiter := NewBandwidthLimiter(1000, 1*time.Second, "", 0)
	defer limiter.Close()

	ip := "192.168.1.1"

	limiter.Allow(ip, 1000)

	// Should drop when over limit (default drop mode)
	if limiter.Allow(ip, 100) {
		t.Error("Should deny when over limit in default (drop) mode")
	}
}

func TestBandwidthLimiter_Cleanup(t *testing.T) {
	window := 50 * time.Millisecond
	limiter := NewBandwidthLimiter(1000, window, "drop", 0)
	defer limiter.Close()

	ip := "192.168.1.1"

	// Create an entry
	limiter.Allow(ip, 100)

	// Check that bucket exists
	limiter.mu.RLock()
	_, exists := limiter.buckets[ip]
	limiter.mu.RUnlock()
	if !exists {
		t.Fatal("Bucket should exist")
	}

	// Wait for cleanup (window * 2 + margin)
	time.Sleep(window*2 + 100*time.Millisecond)

	// Bucket should be cleaned up
	limiter.mu.RLock()
	_, exists = limiter.buckets[ip]
	limiter.mu.RUnlock()
	if exists {
		t.Error("Bucket should be cleaned up")
	}
}

func TestBandwidthLimiter_PartialWindowUsage(t *testing.T) {
	window := 200 * time.Millisecond
	maxBytes := int64(1000)
	limiter := NewBandwidthLimiter(maxBytes, window, "drop", 0)
	defer limiter.Close()

	ip := "192.168.1.1"

	// Use 500 bytes
	if !limiter.Allow(ip, 500) {
		t.Fatal("First 500 bytes should be allowed")
	}

	// Wait half the window
	time.Sleep(window / 2)

	// Use another 500 bytes (should be allowed, total 1000)
	if !limiter.Allow(ip, 500) {
		t.Fatal("Next 500 bytes should be allowed")
	}

	// Should be over limit now
	if limiter.Allow(ip, 100) {
		t.Error("Should be denied when over limit")
	}

	// Wait for first entry to expire
	time.Sleep(window/2 + 50*time.Millisecond)

	// Should have 500 bytes available again
	if !limiter.Allow(ip, 500) {
		t.Error("Should be allowed after first entry expires")
	}
}

func BenchmarkBandwidthLimiter_Allow(b *testing.B) {
	limiter := NewBandwidthLimiter(1000000000, 1*time.Minute, "drop", 0)
	defer limiter.Close()

	ip := "192.168.1.1"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Allow(ip, 1024)
	}
}

func BenchmarkBandwidthLimiter_AllowMultipleIPs(b *testing.B) {
	limiter := NewBandwidthLimiter(1000000000, 1*time.Minute, "drop", 0)
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
		limiter.Allow(ip, 1024)
	}
}

func BenchmarkBandwidthLimiter_ConcurrentAllow(b *testing.B) {
	limiter := NewBandwidthLimiter(1000000000, 1*time.Minute, "drop", 0)
	defer limiter.Close()

	b.RunParallel(func(pb *testing.PB) {
		ip := "192.168.1.1"
		for pb.Next() {
			limiter.Allow(ip, 1024)
		}
	})
}
