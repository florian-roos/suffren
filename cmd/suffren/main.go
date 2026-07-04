package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/florian-roos/suffren/internal/crdt"
	"github.com/florian-roos/suffren/pkg/config"
	"github.com/florian-roos/suffren/pkg/suffren"
)

// CLI entry point for the Suffren application.
func main() {
	peers := map[crdt.NodeId]string{
		"8001": "localhost:8001",
		"8002": "localhost:8002",
		"8003": "localhost:8003",
	}

	port := os.Args[1]
	node := suffren.NewSuffren(crdt.NodeId(port), port, peers, config.DefaultConfig())

	fmt.Printf("Node %s initialized. Press [s] to start\n", port)

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		text := scanner.Text()
		if text == "" {
			continue
		}

		parts := strings.Fields(text)
		switch parts[0] {
		case "s":
			fmt.Printf("Node starting...\n")
			err := node.Start()
			if err != nil {
				fmt.Printf("Failed to start: %v\n", err)
				continue
			}
			fmt.Println("Node started")
			fmt.Println("Commands: [i] <key> [value], [v] <key>, [q]uit")
		case "i":
			if len(parts) < 2 {
				fmt.Println("Usage: i <key> [value]")
				continue
			}
			key := parts[1]
			incValue := uint64(1)
			if len(parts) > 2 {
				parsed, err := strconv.ParseUint(parts[2], 10, 64)
				if err == nil {
					incValue = parsed
				} else {
					fmt.Printf("Invalid increment value: %v\n", err)
					continue
				}
			}
			value, ok := node.IncrementKey(key, incValue)
			if ok {
				fmt.Printf("Incremented key %s. Value: %d\n", key, value)
			} else {
				fmt.Println("Failed to increment.")
			}
		case "v":
			if len(parts) < 2 {
				fmt.Println("Usage: v <key>")
				continue
			}
			key := parts[1]
			value, ok := node.ValueForKey(key)
			if ok {
				fmt.Printf("Value for key %s: %d\n", key, value)
			} else {
				fmt.Println("Failed to get value.")
			}
		case "q":
			node.Stop()
			return
		}
	}
}
