package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/florian-roos/suffren/internal/engine"
)

// CLI entry point for the Suffren application to test it manually on a few nodes
func startInteractiveCLI(node *engine.Engine) {
	fmt.Printf("Node initialized. Press [s] to start\n")

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

	// Check for scanner error
	if err := scanner.Err(); err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input: %v\n", err)
	}
}
