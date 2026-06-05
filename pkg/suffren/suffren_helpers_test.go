package suffren

import (
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/florian-roos/suffren/internal/crdt"
	"github.com/florian-roos/suffren/pkg/config"
)

func init() {
	// Silence the INFO and DEBUG logs for tests and benchmark to not flood the console
	slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError})))
}

// Helpers

// Import config for test
func configForTest() *config.Config {
	cfg := config.DefaultConfig()
	return cfg
}

// Returns a map of 3 nodes with their associated localhost (8001, 8002 and 8003)
func peers3() map[crdt.NodeId]string {
	return map[crdt.NodeId]string{
		"N1": "localhost:8001",
		"N2": "localhost:8002",
		"N3": "localhost:8003",
	}
}

// Returns a map of 3 nodes different from peers3() with their associated localhost (8001, 8002 and 8003)
func peers3bis() map[crdt.NodeId]string {
	return map[crdt.NodeId]string{
		"N1": "localhost:8011",
		"N2": "localhost:8012",
		"N3": "localhost:8013",
	}
}

// startCluster creates and starts all 3 nodes. Returns them and a cleanup func.
func startCluster(tb testing.TB, peers map[crdt.NodeId]string) (s1, s2, s3 *Suffren) {
	tb.Helper()
	cfg := configForTest()

	var ports []string
	var ids []crdt.NodeId
	for id, addr := range peers {
		ids = append(ids, id)
		// extract port from "localhost:PORT"
		ports = append(ports, addr[len("localhost:"):])
	}

	s1 = NewSuffren(ids[0], ports[0], peers, cfg)
	s2 = NewSuffren(ids[1], ports[1], peers, cfg)
	s3 = NewSuffren(ids[2], ports[2], peers, cfg)

	var wg sync.WaitGroup
	for _, s := range []*Suffren{s1, s2, s3} {
		wg.Add(1)
		go func(node *Suffren) {
			defer wg.Done()
			err := node.Start()
			if err != nil {
				tb.Errorf("failed to start node: %v", err)
			}
		}(s)
	}
	wg.Wait()

	tb.Cleanup(func() {
		s1.Stop()
		s2.Stop()
		s3.Stop()
	})

	return s1, s2, s3
}

// waitForConvergence polls until all nodes report the expected value or times out.
func waitForConvergence(tb testing.TB, expected uint64, nodes ...*Suffren) {
	tb.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		allConverged := true
		for _, n := range nodes {
			val, ok := n.Value()
			if !ok || val != expected {
				allConverged = false
				break
			}
		}
		if allConverged {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	for i, n := range nodes {
		val, ok := n.Value()
		tb.Errorf("node %d: expected %d, got %d (ok=%v)", i, expected, val, ok)
	}
	tb.Fatalf("nodes did not converge to %d within 5s", expected)
}