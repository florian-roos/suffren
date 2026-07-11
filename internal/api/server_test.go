package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/florian-roos/suffren/internal/config"
	"github.com/florian-roos/suffren/internal/crdt"
	"github.com/florian-roos/suffren/internal/engine"
	"github.com/florian-roos/suffren/internal/limiter"
	"github.com/florian-roos/suffren/internal/storage"
	"github.com/florian-roos/suffren/internal/testutils"
)

func configForTest() *config.Config {
	return config.DefaultConfig()
}

func peers3() map[crdt.NodeId]string {
	return testutils.GeneratePeers3()
}

func startCluster(tb testing.TB, peers map[crdt.NodeId]string) (s1, s2, s3 *engine.Engine) {
	tb.Helper()
	cfg := configForTest()

	var ids []crdt.NodeId
	for id := range peers {
		ids = append(ids, id)
	}

	s1 = engine.New(ids[0], peers, cfg, &storage.NoopStorage{})
	s2 = engine.New(ids[1], peers, cfg, &storage.NoopStorage{})
	s3 = engine.New(ids[2], peers, cfg, &storage.NoopStorage{})

	var wg sync.WaitGroup
	for _, s := range []*engine.Engine{s1, s2, s3} {
		wg.Add(1)
		go func(node *engine.Engine) {
			defer wg.Done()
			if err := node.Start(); err != nil {
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

func TestNewServer_routesRegistered(t *testing.T) {
	// GIVEN: an API server initialized with a rate limiter
	// WHEN: we make POST requests to the /check and /status endpoints
	// THEN: the server responds without a 404 Not Found error
	s1, _, _ := startCluster(t, peers3())
	limiter := limiter.NewLimiter(s1)
	server := NewServer(limiter)

	ts := httptest.NewServer(server.RouterForTest())
	defer ts.Close()

	// Test POST /check exists (returns 400 for invalid JSON, not 404)
	resp, err := http.Post(ts.URL+"/check", "application/json", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("failed to POST /check: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		t.Fatal("expected /check route to be registered, got 404")
	}

	// Test POST /status exists (returns 400 for invalid JSON, not 404)
	resp, err = http.Post(ts.URL+"/status", "application/json", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatalf("failed to POST /status: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		t.Fatal("expected /status route to be registered, got 404")
	}
}

func TestServer_handleCheck_invalidJSON(t *testing.T) {
	// GIVEN: a running API server
	// WHEN: we send a malformed JSON payload to POST /check
	// THEN: the server responds with a 400 Bad Request status code
	s1, _, _ := startCluster(t, peers3())
	limiter := limiter.NewLimiter(s1)
	server := NewServer(limiter)

	ts := httptest.NewServer(server.RouterForTest())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/check", "application/json", bytes.NewReader([]byte(`not valid json`)))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400 for invalid JSON, got %d", resp.StatusCode)
	}
}

func TestServer_handleCheck_invalidWindow(t *testing.T) {
	// GIVEN: a running API server
	// WHEN: we send a JSON payload with an invalid duration format for the "window" field to POST /check
	// THEN: the server responds with a 400 Bad Request status code
	s1, _, _ := startCluster(t, peers3())
	limiter := limiter.NewLimiter(s1)
	server := NewServer(limiter)

	ts := httptest.NewServer(server.RouterForTest())
	defer ts.Close()

	body := map[string]any{
		"identifier": "user1",
		"resource":   "api",
		"limit":      10,
		"window":     "not-a-duration",
	}
	resp, err := http.Post(ts.URL+"/check", "application/json", bytes.NewReader(mustJSON(t, body)))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400 for invalid window, got %d", resp.StatusCode)
	}
}

func TestServer_handleCheck_allows_under_limit(t *testing.T) {
	// GIVEN: a running API server and a user requesting resources under their rate limit quota
	// WHEN: we make multiple valid requests to POST /check
	// THEN: the server allows the requests and returns the updated quota usage
	s1, _, _ := startCluster(t, peers3())
	limiter := limiter.NewLimiter(s1)
	server := NewServer(limiter)

	ts := httptest.NewServer(server.RouterForTest())
	defer ts.Close()

	body := map[string]any{
		"identifier":      "user_check",
		"resource":        "api",
		"limit":           5,
		"window":          "60s",
		"value_requested": 1,
	}

	for i := 1; i <= 5; i++ {
		resp, err := http.Post(ts.URL+"/check", "application/json", bytes.NewReader(mustJSON(t, body)))
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}

		var checkResp CheckResponse
		if err := json.NewDecoder(resp.Body).Decode(&checkResp); err != nil {
			t.Fatalf("decode response %d failed: %v", i, err)
		}
		_ = resp.Body.Close()

		if !checkResp.Allowed {
			t.Errorf("expected request %d to be allowed", i)
		}
		if checkResp.Current != uint64(i) {
			t.Errorf("expected current to be %d, got %d", i, checkResp.Current)
		}
		if checkResp.Remaining != uint64(5-i) {
			t.Errorf("expected remaining to be %d, got %d", 5-i, checkResp.Remaining)
		}
		if checkResp.Limit != 5 {
			t.Errorf("expected limit to be 5, got %d", checkResp.Limit)
		}
		if checkResp.ResetAt == "" {
			t.Errorf("expected reset_at to be set")
		}
	}
}

func TestServer_handleCheck_denies_over_limit(t *testing.T) {
	// GIVEN: a running API server and a user requesting resources that exceed their rate limit quota
	// WHEN: we make requests that surpass the allowed limit to POST /check
	// THEN: the server denies the excess requests and returns allowed=false
	s1, _, _ := startCluster(t, peers3())
	limiter := limiter.NewLimiter(s1)
	server := NewServer(limiter)

	ts := httptest.NewServer(server.RouterForTest())
	defer ts.Close()

	body := map[string]any{
		"identifier":      "user_deny",
		"resource":        "api",
		"limit":           3,
		"window":          "60s",
		"value_requested": 1,
	}

	// First 3 requests should pass
	for i := 1; i <= 3; i++ {
		resp, err := http.Post(ts.URL+"/check", "application/json", bytes.NewReader(mustJSON(t, body)))
		if err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}

		var checkResp CheckResponse
		_ = json.NewDecoder(resp.Body).Decode(&checkResp)
		_ = resp.Body.Close()

		if !checkResp.Allowed {
			t.Fatalf("expected request %d to be allowed", i)
		}
	}

	// 4th request should be denied
	resp, err := http.Post(ts.URL+"/check", "application/json", bytes.NewReader(mustJSON(t, body)))
	if err != nil {
		t.Fatalf("request 4 failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var checkResp CheckResponse
	if err := json.NewDecoder(resp.Body).Decode(&checkResp); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}

	if checkResp.Allowed {
		t.Error("expected 4th request to be denied")
	}
	if checkResp.Remaining != 0 {
		t.Errorf("expected remaining to be 0, got %d", checkResp.Remaining)
	}
	if checkResp.Current != 3 {
		t.Errorf("expected current to be 3, got %d", checkResp.Current)
	}
}

func TestServer_handleCheck_contentType(t *testing.T) {
	// GIVEN: a running API server
	// WHEN: we make a valid request to POST /check
	// THEN: the server responds with the "application/json" Content-Type header
	s1, _, _ := startCluster(t, peers3())
	limiter := limiter.NewLimiter(s1)
	server := NewServer(limiter)

	ts := httptest.NewServer(server.RouterForTest())
	defer ts.Close()

	body := map[string]any{
		"identifier":      "user_ct",
		"resource":        "api",
		"limit":           10,
		"window":          "60s",
		"value_requested": 1,
	}

	resp, err := http.Post(ts.URL+"/check", "application/json", bytes.NewReader(mustJSON(t, body)))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type to be application/json, got %s", ct)
	}
}

func TestServer_handleStatus_returns_current_without_incrementing(t *testing.T) {
	// GIVEN: a running API server with some previously consumed rate limit quota
	// WHEN: we make multiple requests to POST /status
	// THEN: the server returns the current consumption without incrementing it
	s1, _, _ := startCluster(t, peers3())
	limiter := limiter.NewLimiter(s1)
	server := NewServer(limiter)

	ts := httptest.NewServer(server.RouterForTest())
	defer ts.Close()

	// First, make a check to increment
	checkBody := map[string]any{
		"identifier":      "user_status",
		"resource":        "api",
		"limit":           10,
		"window":          "60s",
		"value_requested": 3,
	}
	resp, err := http.Post(ts.URL+"/check", "application/json", bytes.NewReader(mustJSON(t, checkBody)))
	if err != nil {
		t.Fatalf("check request failed: %v", err)
	}
	_ = resp.Body.Close()

	// Now call status
	statusBody := map[string]any{
		"identifier": "user_status",
		"resource":   "api",
		"limit":      10,
		"window":     "60s",
	}

	resp, err = http.Post(ts.URL+"/status", "application/json", bytes.NewReader(mustJSON(t, statusBody)))
	if err != nil {
		t.Fatalf("status request failed: %v", err)
	}

	var statusResp StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&statusResp); err != nil {
		t.Fatalf("decode status response failed: %v", err)
	}
	_ = resp.Body.Close()

	if statusResp.Current != 3 {
		t.Errorf("expected status current to be 3, got %d", statusResp.Current)
	}
	if statusResp.Remaining != 7 {
		t.Errorf("expected remaining to be 7, got %d", statusResp.Remaining)
	}

	// Call status again to verify it didn't increment
	resp, err = http.Post(ts.URL+"/status", "application/json", bytes.NewReader(mustJSON(t, statusBody)))
	if err != nil {
		t.Fatalf("status request 2 failed: %v", err)
	}

	var statusResp2 StatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&statusResp2); err != nil {
		t.Fatalf("decode status response 2 failed: %v", err)
	}
	_ = resp.Body.Close()

	if statusResp2.Current != 3 {
		t.Errorf("expected status current to still be 3, got %d", statusResp2.Current)
	}
}

func TestServer_handleStatus_invalidJSON(t *testing.T) {
	// GIVEN: a running API server
	// WHEN: we send a malformed JSON payload to POST /status
	// THEN: the server responds with a 400 Bad Request status code
	s1, _, _ := startCluster(t, peers3())
	limiter := limiter.NewLimiter(s1)
	server := NewServer(limiter)

	ts := httptest.NewServer(server.RouterForTest())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/status", "application/json", bytes.NewReader([]byte(`not valid json`)))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400 for invalid JSON, got %d", resp.StatusCode)
	}
}

func TestServer_handleStatus_invalidWindow(t *testing.T) {
	// GIVEN: a running API server
	// WHEN: we send a JSON payload with an invalid duration format for the "window" field to POST /status
	// THEN: the server responds with a 400 Bad Request status code
	s1, _, _ := startCluster(t, peers3())
	limiter := limiter.NewLimiter(s1)
	server := NewServer(limiter)

	ts := httptest.NewServer(server.RouterForTest())
	defer ts.Close()

	body := map[string]any{
		"identifier": "user1",
		"resource":   "api",
		"limit":      10,
		"window":     "not-a-duration",
	}
	resp, err := http.Post(ts.URL+"/status", "application/json", bytes.NewReader(mustJSON(t, body)))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400 for invalid window, got %d", resp.StatusCode)
	}
}

func TestServer_handleStatus_contentType(t *testing.T) {
	// GIVEN: a running API server
	// WHEN: we make a valid request to POST /status
	// THEN: the server responds with the "application/json" Content-Type header
	s1, _, _ := startCluster(t, peers3())
	limiter := limiter.NewLimiter(s1)
	server := NewServer(limiter)

	ts := httptest.NewServer(server.RouterForTest())
	defer ts.Close()

	body := map[string]any{
		"identifier": "user_ct",
		"resource":   "api",
		"limit":      10,
		"window":     "60s",
	}

	resp, err := http.Post(ts.URL+"/status", "application/json", bytes.NewReader(mustJSON(t, body)))
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type to be application/json, got %s", ct)
	}
}

func TestServer_methodNotAllowed(t *testing.T) {
	// GIVEN: a running API server
	// WHEN: we make GET requests to the /check or /status endpoints
	// THEN: the server responds with a 405 Method Not Allowed status code
	s1, _, _ := startCluster(t, peers3())
	limiter := limiter.NewLimiter(s1)
	server := NewServer(limiter)

	ts := httptest.NewServer(server.RouterForTest())
	defer ts.Close()

	// GET /check should return 405 Method Not Allowed
	resp, err := http.Get(ts.URL + "/check")
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected GET /check to return 405, got %d", resp.StatusCode)
	}

	// GET /status should return 405 Method Not Allowed
	resp, err = http.Get(ts.URL + "/status")
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected GET /status to return 405, got %d", resp.StatusCode)
	}
}

func TestServer_notFound(t *testing.T) {
	// GIVEN: a running API server
	// WHEN: we make a request to an unregistered endpoint
	// THEN: the server responds with a 404 Not Found status code
	s1, _, _ := startCluster(t, peers3())
	limiter := limiter.NewLimiter(s1)
	server := NewServer(limiter)

	ts := httptest.NewServer(server.RouterForTest())
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/unknown")
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected GET /unknown to return 404, got %d", resp.StatusCode)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("failed to marshal JSON: %v", err)
	}
	return b
}
