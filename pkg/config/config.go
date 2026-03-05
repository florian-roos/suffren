package config

import (
	"encoding/json"
	"os"
	"time"
)

type Config struct {
	Node             NodeConfig             `json:"node"`
	LatticeAgreement LatticeAgreementConfig `json:"lattice_agreement"`
}

type NodeConfig struct {
	// ProposalInterval is how often the node checks if a new round is needed.
	// Default: 50ms
	ProposalInterval time.Duration

	// RoundTimeout is how long a proposal round may be in flight before
	// it is considered dead and a new round is started.
	// Must be > ProposalInterval.
	// Default: 2s
	RoundTimeout time.Duration
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
		Node: NodeConfig{
			ProposalInterval: 50 * time.Millisecond,
			RoundTimeout:     2 * time.Second,
		},
		LatticeAgreement: LatticeAgreementConfig{
			MsgChanSize: 1024,
		},
	}
}
