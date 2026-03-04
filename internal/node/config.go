package node

import "time"

type Config struct {
	// ProposalInterval is how often the node checks if a new round is needed.
	// Default: 50ms
	ProposalInterval time.Duration

	// RoundTimeout is how long a proposal round may be in flight before
	// it is considered dead and a new round is started.
	// Must be > ProposalInterval.
	// Default: 2s
	RoundTimeout time.Duration
}

func DefaultConfig() Config {
	return Config{
		ProposalInterval: 50 * time.Millisecond,
		RoundTimeout:     2 * time.Second,
	}
}
