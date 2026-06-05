package crdt

import (
	"fmt"
	"testing"
)

func BenchmarkGCounter_MergeInPlace(b *testing.B) {
	nodeIds := []NodeId{"N1", "N2", "N3", "N4", "N5"}
	g1 := NewGCounter(nodeIds)
	g2 := NewGCounter(nodeIds)

	for i, id := range nodeIds {
		g1.Counts[id] = uint64(i)
		g2.Counts[id] = uint64(len(nodeIds) - i)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// We change at each iteration the value of g2 to create a different join result, to avoid any caching effect that could make the benchmark less representative.
		for id := range g2.Counts {
			g2.Counts[id]++
		}

		g1.MergeInPlace(g2)
	}
}

// Benchmark Join that creates a new GCounter, to compare with MergeInPlace that modifies in place.
func BenchmarkGCounter_Join(b *testing.B) {
	nodeIds := []NodeId{"N1", "N2", "N3", "N4", "N5"}
	g1 := NewGCounter(nodeIds)
	g2 := NewGCounter(nodeIds)

	for i, id := range nodeIds {
		g1.Counts[id] = uint64(i)
		g2.Counts[id] = uint64(len(nodeIds) - i)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// We change at each iteration the value of g2 to create a different join result, to avoid any caching effect that could make the benchmark less representative.
		for id := range g2.Counts {
			g2.Counts[id]++
		}
		_ = g1.Join(g2)
	}
}

// Benchmark Copy to measure the cost of copying a GCounter, which is done at each proposal in Suffren.
func BenchmarkGCounter_Copy(b *testing.B) {
	sizes := []int{10, 100, 1000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("Size_%d", size), func(b *testing.B) {
			var nodeIds []NodeId
			for i := 0; i < size; i++ {
				nodeIds = append(nodeIds, NodeId(fmt.Sprintf("N%d", i)))
			}
			g := NewGCounter(nodeIds)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = g.Copy()
			}
		})
	}
}
