package engine

import (
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/florian-roos/suffren/internal/config"
	"github.com/florian-roos/suffren/internal/crdt"
	"github.com/florian-roos/suffren/internal/storage"
	"github.com/florian-roos/suffren/internal/testutils"
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

// Returns a map of 3 nodes with their associated localhost (dynamic ports)
func peers3() map[crdt.NodeId]string {
	return testutils.GeneratePeers3()
}

// Returns a map of 3 nodes different from peers3() with their associated localhost (dynamic ports)
func peers3bis() map[crdt.NodeId]string {
	return peers3() // Now it's dynamic so we can just reuse peers3()!
}

// startCluster creates and starts all 3 nodes. Returns them and a cleanup func.
func startCluster(tb testing.TB, peers map[crdt.NodeId]string) (s1, s2, s3 *Engine) {
	tb.Helper()
	cfg := configForTest()

	var ids []crdt.NodeId
	for id := range peers {
		ids = append(ids, id)
	}

	s1 = New(ids[0], peers, cfg, &storage.NoopStorage{})
	s2 = New(ids[1], peers, cfg, &storage.NoopStorage{})
	s3 = New(ids[2], peers, cfg, &storage.NoopStorage{})

	var wg sync.WaitGroup
	for _, s := range []*Engine{s1, s2, s3} {
		wg.Add(1)
		go func(node *Engine) {
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
func waitForConvergence(tb testing.TB, key string, expected uint64, nodes ...*Engine) {
	tb.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		allConverged := true
		for _, n := range nodes {
			val, ok := n.ValueForKey(key)
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
		val, ok := n.ValueForKey(key)
		tb.Errorf("node %d: expected %d, got %d (ok=%v)", i, expected, val, ok)
	}
	tb.Fatalf("nodes did not converge to %d within 5s", expected)
}
