package main

import (
	"bufio"
	"fmt"
	"os"
	"suffren/internal/crdt"
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
	suffren, err := suffren.NewSuffren(crdt.NodeId(port), port, peers)
	if err != nil {
		fmt.Printf("Error starting Suffren: %v\n", err)
		os.Exit(1)
	}
	defer suffren.Stop()

	fmt.Printf("Suffren node started on port %s. Commands: [i]ncrement, [v]alue, [q]uit\n", port)

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		switch scanner.Text() {
		case "i":
			suffren.Increment()
			fmt.Println("Counter incremented. Local value:", suffren.Value())
		case "v":
			fmt.Println("Local value:", suffren.Value())
		case "q":
			return
		}
	}
}
