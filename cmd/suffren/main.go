package main

import (
	"flag"
	"log/slog"
	"os"
	"strings"

	"github.com/florian-roos/suffren/internal/api"
	"github.com/florian-roos/suffren/internal/crdt"
	"github.com/florian-roos/suffren/internal/ratelimiter"
	"github.com/florian-roos/suffren/pkg/config"
	"github.com/florian-roos/suffren/pkg/suffren"
	"github.com/joho/godotenv"
)

func main() {
	runAPI := flag.Bool("api", false, "Start the API HTTP server of Suffren Watchguard")
	nodeId := flag.String("id", "N1", "Unique identifier of the node in the cluster (ex: N1)")
	apiPort := flag.String("api-port", "8080", "Listening port for the HTTP API (ex: 8081)")
	flag.Parse()

	err := godotenv.Load()
	if err != nil {
		slog.Error("No .env file found", "error", err)
	}

	peers := parseStringToPeersMap(os.Getenv("PEERS"))

	node := suffren.NewSuffren(crdt.NodeId(*nodeId), peers, config.DefaultConfig())
	limiter := ratelimiter.NewLimiter(node)
	apiServer := api.NewServer(limiter)

	if *runAPI {
		// Run the production API server
		err := node.Start()
		if err != nil {
			slog.Error("Failed to start suffren node", "error", err)
		}

		err = apiServer.Start("localhost:" + *apiPort)
		if err != nil {
			slog.Error("Failed to start API server", "error", err)
		}
	} else {
		// Run the CLI to test suffren on a few nodes
		startInteractiveCLI(node)
	}
}

// returns the peers map from the string in the .env file (format : "N1:localhost:8031,N2:localhost:8032")
func parseStringToPeersMap(s string) map[crdt.NodeId]string {
	if s == "" {
		slog.Error("PEERS is not defined in .env")
	}

	peers := make(map[crdt.NodeId]string)

	peersSlice := strings.Split(s, ",")
	for _, pair := range peersSlice {
		parts := strings.SplitN(pair, ":", 2)
		if len(parts) == 2 {
			nodeId := parts[0]
			address := parts[1]
			peers[crdt.NodeId(nodeId)] = address
		}
	}
	return peers
}
