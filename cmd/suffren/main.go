package main

import (
	"flag"
	"log/slog"
	"os"
	"strings"

	"github.com/florian-roos/suffren/internal/api"
	"github.com/florian-roos/suffren/internal/config"
	"github.com/florian-roos/suffren/internal/crdt"
	"github.com/florian-roos/suffren/internal/engine"
	"github.com/florian-roos/suffren/internal/limiter"
	"github.com/florian-roos/suffren/internal/storage"
	"github.com/joho/godotenv"
)

func main() {
	runAPI := flag.Bool("api", false, "Start the API HTTP server of Suffren Watchguard")
	nodeIdFlag := flag.String("id", "", "Unique identifier of the node in the cluster (ex: N1)")
	apiPort := flag.String("api-port", "8080", "Listening port for the HTTP API (ex: 8081)")
	flag.Parse()

	// Use the region as nodeID in Fly.io
	nodeId := *nodeIdFlag
	if nodeId == "" {
		if flyRegion := os.Getenv("FLY_REGION"); flyRegion != "" {
			nodeId = flyRegion
		} else {
			nodeId = "N1"
		}
	}

	err := godotenv.Load()
	if err != nil {
		slog.Error("No .env file found", "error", err)
	}

	peers := parseStringToPeersMap(os.Getenv("PEERS"))
	setupLogger()
	slog.SetDefault(slog.Default().With("node_id", nodeId))

	cfg := config.DefaultConfig()

	dataPath := os.Getenv("DATA_PATH")
	if dataPath == "" {
		dataPath = "./data"
	}
	store := storage.NewFileStorage(dataPath + "/suffren_" + nodeId + ".json")

	node := engine.New(crdt.NodeId(nodeId), peers, cfg, store)
	limiter := limiter.NewLimiter(node)
	apiServer := api.NewServer(limiter)

	if *runAPI {
		// Run the production API server
		err := node.Start()
		if err != nil {
			slog.Error("Failed to start suffren node", "error", err)
		}

		err = apiServer.Start("[::]:" + *apiPort)
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

// configures the global slog instance based on the configuration.
func setupLogger() {
	var level slog.Level

	logLevelStr := os.Getenv("LOG_LEVEL")
	if logLevelStr == "" {
		logLevelStr = "INFO"
	}

	switch strings.ToUpper(logLevelStr) {
	case "DEBUG":
		level = slog.LevelDebug
	case "WARN":
		level = slog.LevelWarn
	case "ERROR":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	logFormatStr := os.Getenv("LOG_FORMAT")
	if logFormatStr == "" {
		logFormatStr = "text"
	}

	var handler slog.Handler
	if strings.ToLower(logFormatStr) == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	slog.SetDefault(slog.New(handler))
}
