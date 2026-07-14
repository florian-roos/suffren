package config

import (
	"encoding/json"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Engine           EngineConfig           `json:"engine"`
	LatticeAgreement LatticeAgreementConfig `json:"lattice_agreement"`
}

type EngineConfig struct {
	// RoundTimeout is how long a proposal round may be in flight before
	// it is considered dead and a new round is started.
	// Must be > ProposalInterval.
	// Default: 2s
	RoundTimeout       time.Duration
	StartupSyncTimeout time.Duration // StartupSyncTimeout is the maximum time to retry Value() on startup
	BatchTimeout       time.Duration // BatchTimeout is the time we wait before a batch is proposed to the network
	MaxBatchSize       int
	SnapshotInterval   time.Duration // SnapshotInterval is the time between automatic background disk snapshots
}

type LatticeAgreementConfig struct {
	MsgChanSize int
}

func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var cfg Config
	err = json.Unmarshal(data, &cfg)
	if err != nil {
		return nil, err
	}

	return &cfg, nil
}

func DefaultConfig() *Config {
	cfg := &Config{
		Engine: EngineConfig{
			RoundTimeout:       2 * time.Second,
			StartupSyncTimeout: 20 * time.Second,
			BatchTimeout:       1 * time.Millisecond,
			MaxBatchSize:       100,
			SnapshotInterval:   5 * time.Second,
		},
		LatticeAgreement: LatticeAgreementConfig{
			MsgChanSize: 1024,
		},
	}

	if envBatchTimeout := os.Getenv("SUFFREN_BATCH_TIMEOUT"); envBatchTimeout != "" {
		if d, err := time.ParseDuration(envBatchTimeout); err == nil {
			cfg.Engine.BatchTimeout = d
		}
	}

	if envBatchSize := os.Getenv("SUFFREN_BATCH_SIZE"); envBatchSize != "" {
		if size, err := strconv.Atoi(envBatchSize); err == nil {
			cfg.Engine.MaxBatchSize = size
		}
	}

	return cfg
}
