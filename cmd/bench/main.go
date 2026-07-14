package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

type Result struct {
	Duration   time.Duration
	StatusCode int
	Err        error
}

func main() {
	var (
		nRequests   = flag.Int("n", 1000, "Number of requests to run.")
		concurrency = flag.Int("c", 50, "Number of workers to run concurrently.")
		targetUrls  = flag.String("url", "http://localhost:8081/check", "Comma-separated list of URLs to benchmark.")
		randomId    = flag.Bool("random-id", false, "Use random identifiers to avoid CRDT contention.")
		keyRange    = flag.Int("key-range", 1000000, "Range of random keys to generate when random-id is true.")
	)
	flag.Parse()

	urls := strings.Split(*targetUrls, ",")
	for i := range urls {
		urls[i] = strings.TrimSpace(urls[i])
	}

	fmt.Printf("Running %d requests with concurrency %d to %d nodes\n\n", *nRequests, *concurrency, len(urls))

	jobs := make(chan struct{}, *nRequests)
	results := make(chan Result, *nRequests)

	var wg sync.WaitGroup
	wg.Add(*concurrency)

	// We use a custom HTTP Transport to avoid TCP port starvation.
	// By default, Go's HTTP client only keeps 2 idle connections per host.
	// Since we are running high concurrency, we need to allow as many idle connections
	// as our concurrency level, so we reuse existing TCP sockets instead of creating new ones.
	tr := &http.Transport{
		MaxIdleConns:        *concurrency,
		MaxIdleConnsPerHost: *concurrency,
	}
	client := &http.Client{
		Transport: tr,
		Timeout:   10 * time.Second,
	}

	// Pre-compile the static payload to avoid burning CPU with json.Marshal
	// 200,000 times inside the hot loop, which would artificially bottleneck the client!
	var staticPayload []byte
	if !*randomId {
		payload := map[string]interface{}{
			"identifier":      "bench_user",
			"resource":        "api_bench",
			"limit":           10000000,
			"window":          "60s",
			"value_requested": 1,
		}
		staticPayload, _ = json.Marshal(payload)
	}

	start := time.Now()

	// Spawn workers
	for i := 0; i < *concurrency; i++ {
		go func() {
			defer wg.Done()
			for range jobs {
				var bodyBytes []byte
				if *randomId {
					id := fmt.Sprintf("user_%d", rand.Intn(*keyRange))
					payload := map[string]interface{}{
						"identifier":      id,
						"resource":        "api_bench",
						"limit":           10000000,
						"window":          "60s",
						"value_requested": 1,
					}
					bodyBytes, _ = json.Marshal(payload)
				} else {
					// Use the pre-computed bytes to maximize client HTTP throughput
					bodyBytes = staticPayload
				}

				url := urls[rand.Intn(len(urls))]

				reqStart := time.Now()
				req, _ := http.NewRequest("POST", url, bytes.NewBuffer(bodyBytes))
				req.Header.Set("Content-Type", "application/json")

				resp, err := client.Do(req)
				duration := time.Since(reqStart)

				if err != nil {
					results <- Result{Duration: duration, Err: err}
					continue
				}

				// CRITICAL: We must read the entire response body and close it,
				// even if we don't care about the content. If we don't do this,
				// the Go HTTP client cannot reuse the underlying TCP connection!
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()

				results <- Result{Duration: duration, StatusCode: resp.StatusCode}
			}
		}()
	}

	// Feed the jobs queue
	for i := 0; i < *nRequests; i++ {
		jobs <- struct{}{}
	}
	close(jobs)

	// Wait for completion
	wg.Wait()
	close(results)
	totalDuration := time.Since(start)

	// Process results
	var allDurations []time.Duration
	statusCounts := make(map[int]int)
	errCounts := make(map[string]int)
	var totalReqDuration time.Duration

	for res := range results {
		allDurations = append(allDurations, res.Duration)
		totalReqDuration += res.Duration
		if res.Err != nil {
			errCounts[res.Err.Error()]++
		} else {
			statusCounts[res.StatusCode]++
		}
	}

	sort.Slice(allDurations, func(i, j int) bool {
		return allDurations[i] < allDurations[j]
	})

	fastest := allDurations[0]
	slowest := allDurations[len(allDurations)-1]
	average := totalReqDuration / time.Duration(len(allDurations))
	reqPerSec := float64(*nRequests) / totalDuration.Seconds()

	// Summary
	fmt.Println("Summary:")
	fmt.Printf("  Total:\t%.4f secs\n", totalDuration.Seconds())
	fmt.Printf("  Slowest:\t%.4f secs\n", slowest.Seconds())
	fmt.Printf("  Fastest:\t%.4f secs\n", fastest.Seconds())
	fmt.Printf("  Average:\t%.4f secs\n", average.Seconds())
	fmt.Printf("  Requests/sec:\t%.4f\n\n", reqPerSec)

	// Response time histogram
	printHistogram(allDurations)

	// Latency distribution
	fmt.Println("Latency distribution:")
	fmt.Printf("  10%% in %.4f secs\n", getPercentile(allDurations, 0.10).Seconds())
	fmt.Printf("  25%% in %.4f secs\n", getPercentile(allDurations, 0.25).Seconds())
	fmt.Printf("  50%% in %.4f secs\n", getPercentile(allDurations, 0.50).Seconds())
	fmt.Printf("  75%% in %.4f secs\n", getPercentile(allDurations, 0.75).Seconds())
	fmt.Printf("  90%% in %.4f secs\n", getPercentile(allDurations, 0.90).Seconds())
	fmt.Printf("  95%% in %.4f secs\n", getPercentile(allDurations, 0.95).Seconds())
	fmt.Printf("  99%% in %.4f secs\n\n", getPercentile(allDurations, 0.99).Seconds())

	// Status codes
	fmt.Println("Status code distribution:")
	for code, count := range statusCounts {
		fmt.Printf("  [%d]\t%d responses\n", code, count)
	}

	if len(errCounts) > 0 {
		fmt.Println("\nError distribution:")
		for errStr, count := range errCounts {
			fmt.Printf("  %d times: %s\n", count, errStr)
		}
	}
}

// getPercentile calculates the duration at a specific percentile (e.g. 0.99 for p99).
// The input slice must be sorted in ascending order.
func getPercentile(sorted []time.Duration, p float64) time.Duration {
	index := int(math.Ceil(float64(len(sorted))*p)) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

// printHistogram generates an ASCII art bar chart showing the distribution of latencies.
// It groups the durations into 10 buckets and scales the bars to a maximum width of 40 characters.
func printHistogram(sorted []time.Duration) {
	if len(sorted) == 0 {
		return
	}
	fmt.Println("Response time histogram:")

	min := sorted[0].Seconds()
	max := sorted[len(sorted)-1].Seconds()
	if min == max {
		max = min + 0.001
	}

	bins := 10
	step := (max - min) / float64(bins)

	counts := make([]int, bins)
	for _, d := range sorted {
		sec := d.Seconds()
		idx := int((sec - min) / step)
		if idx == bins {
			idx--
		}
		counts[idx]++
	}

	maxCount := 0
	for _, c := range counts {
		if c > maxCount {
			maxCount = c
		}
	}

	for i := 0; i < bins; i++ {
		binEnd := min + float64(i+1)*step
		barLen := 0
		if maxCount > 0 {
			barLen = int(math.Round(float64(counts[i]) / float64(maxCount) * 40)) // 40 max chars
		}
		bar := strings.Repeat("■", barLen)
		fmt.Printf("  %.3f [%d]\t|%s\n", binEnd, counts[i], bar)
	}
	fmt.Println()
}
