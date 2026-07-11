package main

import (
	"fmt"
	"time"
)

func main() {
	now, _ := time.Parse(time.RFC3339, "2026-07-11T13:39:00Z")
	window := 6000 * time.Second

	currentWindowStart := now.Truncate(window)
	previousWindowStart := currentWindowStart.Add(-window)

	fmt.Printf("Now: %v\n", now)
	fmt.Printf("Current Window Start: %v\n", currentWindowStart)
	fmt.Printf("Previous Window Start: %v\n", previousWindowStart)
}
