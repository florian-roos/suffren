package config

import (
	"encoding/json"
	"os"
	"time"
)

type Config struct {
	Engine           EngineConfig           `json:"engine"`
	LatticeAgreement LatticeAgreementConfig `json:"lattice_agreement"`
	LogLevel         string                 `json:"log_level"`
	LogFormat        string                 `json:"log_format"`
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
	return &Config{
		Engine: EngineConfig{
			RoundTimeout:       2 * time.Second,
			StartupSyncTimeout: 20 * time.Second,
			BatchTimeout:       10 * time.Millisecond,
			MaxBatchSize:       100,
		},
		LatticeAgreement: LatticeAgreementConfig{
			MsgChanSize: 1024,
		},
		LogLevel:  "INFO",
		LogFormat: "text",
	}
}
