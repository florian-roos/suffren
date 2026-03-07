package main

import (
	"bufio"
	"fmt"
	"os"
	"suffren/internal/crdt"
	"suffren/pkg/config"
	"suffren/pkg/suffren"
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

	fmt.Printf("Node %s initialized. Commands: [s]tart, [i]ncrement, [v]alue, [q]uit\n", port)

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		switch scanner.Text() {
		case "s":
			err := node.Start()
			if err != nil {
				fmt.Printf("Failed to start: %v\n", err)
				continue
			}
			fmt.Println("Node started and connected to cluster.")
		case "i":
			value, ok := node.Increment()
			if ok {
				fmt.Println("Incremented. Value:", value)
			} else {
				fmt.Println("Failed to increment.")
			}
		case "v":
			value, ok := node.Value()
			if ok {
				fmt.Println("Value:", value)
			} else {
				fmt.Println("Failed to get value.")
			}
		case "q":
			node.Stop()
			return
		}
	}
}
