package testutils

import (
	"fmt"
	"math/rand"
	"sync/atomic"

	"github.com/florian-roos/suffren/internal/crdt"
)

var portCounter int32 = int32(10000 + rand.Intn(40000))

// GeneratePeers3 returns a map of 3 nodes with their associated localhost on dynamic ports.
// This prevents port collision when running tests in parallel across packages.
func GeneratePeers3() map[crdt.NodeId]string {
	p1 := atomic.AddInt32(&portCounter, 1)
	p2 := atomic.AddInt32(&portCounter, 1)
	p3 := atomic.AddInt32(&portCounter, 1)
	return map[crdt.NodeId]string{
		"N1": fmt.Sprintf("localhost:%d", p1),
		"N2": fmt.Sprintf("localhost:%d", p2),
		"N3": fmt.Sprintf("localhost:%d", p3),
	}
}
