package suffren

import (
	"testing"
)

// BenchmarkSoloIncrement measure the throughput when one node increments a key in a solo run
func BenchmarkSoloIncrement(b *testing.B) {
	peers := peers3bis()
	s1, _, _ := startCluster(b, peers)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, ok := s1.IncrementKey("bench_key", 1)
		if !ok {
			b.Fatalf("Increment %d timed out", i)
		}
	}
}

// BenchmarkConcurrentLocalIncrements measures the throughput when a lot of goroutines are calling increment on single node on teh same key
// It tests the contention of the opMu mutex
func BenchmarkConcurrentLocalIncrements(b *testing.B) {
	peers := peers3bis()
	s1, _, _ := startCluster(b, peers)

	b.ResetTimer()
	// RunParallel exexutes the function in multiple goroutines
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, ok := s1.IncrementKey("bench_key", 1)
			if !ok {
				b.Error("Increment timed out")
			}
		}
	})
}

// BenchmarkClusterIncrements measures the throughput of the entire cluster when a lot of goroutines increment every nodes in parallel.
func BenchmarkClusterIncrements(b *testing.B) {
	peers := peers3bis()
	s1, s2, s3 := startCluster(b, peers)
	nodes := []*Suffren{s1, s2, s3}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			node := nodes[i%3]
			_, ok := node.IncrementKey("bench_key", 1)
			if !ok {
				b.Error("Increment timed out")
			}
			i++
		}
	})
}

// BenchmarkValueRead mesures the time to read in a non-concurrent network of 3 nodes
func BenchmarkValueForKey(b *testing.B) {
	peers := peers3bis()
	s1, _, _ := startCluster(b, peers)

	// We increment one time to have a value
	s1.IncrementKey("bench_key", 1)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, ok := s1.ValueForKey("bench_key")
		if !ok {
			b.Fatalf("Value %d timed out", i)
		}
	}
}

// BenchmarkClusterValueForKey measures the throughput of the entire cluster when a lot of goroutines request a value for a key on every nodes in parallel.
func BenchmarkClusterValueForKey(b *testing.B) {
	peers := peers3bis()
	s1, s2, s3 := startCluster(b, peers)
	nodes := []*Suffren{s1, s2, s3}

	// We increment one time to have a value
	s1.IncrementKey("bench_key", 1)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			node := nodes[i%3]
			_, ok := node.ValueForKey("bench_key")
			if !ok {
				b.Error("Increment timed out")
			}
			i++
		}
	})
}
