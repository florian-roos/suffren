package suffren

import (
	"testing"
)

func BenchmarkSoloIncrement(b *testing.B) {
	peers := peers3bis()
	s1, _, _ := startCluster(b, peers) 

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, ok := s1.Increment()
		if !ok {
			b.Fatalf("Increment %d timed out", i)
		}
	}
}

// BenchmarkConcurrentLocalIncrements measures the throughput when a lot of goroutines are calling increment on single node
// It tests the contention of the opMu mutex
func BenchmarkConcurrentLocalIncrements(b *testing.B) {
	peers := peers3bis()
	s1, _, _ := startCluster(b, peers)

	b.ResetTimer()
	// RunParallel exexutes the function in multiple goroutines
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, ok := s1.Increment()
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
			_, ok := node.Increment()
			if !ok {
				b.Error("Increment timed out")
			}
			i++
		}
	})
}

// BenchmarkValueRead mesures the time to read in a non-concurrent network of 3 nodes
func BenchmarkValueRead(b *testing.B) {
	peers := peers3bis()
	s1, _, _ := startCluster(b, peers)

	// We incremente one time to have a value
	s1.Increment()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, ok := s1.Value()
		if !ok {
			b.Fatalf("Value %d timed out", i)
		}
	}
}