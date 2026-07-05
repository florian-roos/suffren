package ratelimiter

import (
	"testing"
	"time"
)

// BenchmarkLimiter_SingleNode_Sequential measures the throughput of consecutive checks on a single node.
func BenchmarkLimiter_SingleNode_Sequential(b *testing.B) {
	s1, _, _ := startCluster(b, peers3())
	limiter := NewLimiter(s1)

	// Infinite limit to avoid hitting the block condition, we just want to benchmark the engine.
	rule := Rule{Limit: ^uint64(0), Window: 1 * time.Minute}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dec := limiter.Check("bench_user", "api", 1, rule)
		if dec.Error != nil {
			b.Fatalf("Check failed: %v", dec.Error)
		}
	}
}

// BenchmarkLimiter_SingleNode_Concurrent measures the throughput when many goroutines concurrently check the limit on a single node.
// Tests the lock contention inside the node (s.mu).
func BenchmarkLimiter_SingleNode_Concurrent(b *testing.B) {
	s1, _, _ := startCluster(b, peers3())
	limiter := NewLimiter(s1)
	rule := Rule{Limit: ^uint64(0), Window: 1 * time.Minute}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			dec := limiter.Check("bench_user_concurrent", "api", 1, rule)
			if dec.Error != nil {
				b.Errorf("Check failed: %v", dec.Error)
			}
		}
	})
}

// BenchmarkLimiter_Cluster_Concurrent measures the throughput when load is distributed across the entire cluster.
// Tests the network consensus overhead under high load.
func BenchmarkLimiter_Cluster_Concurrent(b *testing.B) {
	s1, s2, s3 := startCluster(b, peers3())
	limiters := []*Limiter{NewLimiter(s1), NewLimiter(s2), NewLimiter(s3)}
	rule := Rule{Limit: ^uint64(0), Window: 1 * time.Minute}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			limiter := limiters[i%3]
			dec := limiter.Check("bench_cluster_user", "api", 1, rule)
			if dec.Error != nil {
				b.Errorf("Check failed: %v", dec.Error)
			}
			i++
		}
	})
}
