package suffren

import (
	"testing"
)

func BenchmarkSoloIncrement(b *testing.B) {
	peers := peers3()
	s1, _, _ := startCluster(b, peers)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, ok := s1.Increment()
		if !ok{
			b.Fatalf("Increment %d timed out", i)
		}
	} 
}