package node

import "time"

type Config struct {
	ProposalInterval time.Duration
}

func DefaultConfig() Config {
	return Config{
		ProposalInterval: 500 * time.Millisecond,
	}
}
