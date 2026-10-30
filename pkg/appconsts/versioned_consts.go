package appconsts

import "time"

// GetUpgradeHeightDelay returns the delay in blocks after a quorum has been
// reached that the chain should upgrade to the new version.
func GetUpgradeHeightDelay(chainID string) int64 {
	if chainID == TestChainID {
		return TestUpgradeHeightDelay
	}
	if chainID == CortoChainID {
		return CortoUpgradeHeightDelay
	}
	if chainID == MochaChainID {
		return MochaUpgradeHeightDelay
	}
	return MainnetUpgradeHeightDelay
}

// ConsensusTimeouts are the consensus timeouts the app returns to CometBFT.
type ConsensusTimeouts struct {
	TimeoutPropose          time.Duration
	TimeoutProposeDelta     time.Duration
	TimeoutPrevote          time.Duration
	TimeoutPrevoteDelta     time.Duration
	TimeoutPrecommit        time.Duration
	TimeoutPrecommitDelta   time.Duration
	TimeoutCommit           time.Duration
	DelayedPrecommitTimeout time.Duration
}

// v10ConsensusTimeouts give a ~2.6s block: DelayedPrecommitTimeout + TimeoutCommit.
var v10ConsensusTimeouts = ConsensusTimeouts{
	TimeoutPropose:          3000 * time.Millisecond,
	TimeoutProposeDelta:     500 * time.Millisecond,
	TimeoutPrevote:          2000 * time.Millisecond,
	TimeoutPrevoteDelta:     500 * time.Millisecond,
	TimeoutPrecommit:        3000 * time.Millisecond,
	TimeoutPrecommitDelta:   500 * time.Millisecond,
	TimeoutCommit:           500 * time.Millisecond,
	DelayedPrecommitTimeout: 2100 * time.Millisecond,
}

// v11ConsensusTimeouts make TimeoutCommit a floor on the block interval.
// celestia-core no longer delays precommits, so DelayedPrecommitTimeout is 0.
var v11ConsensusTimeouts = ConsensusTimeouts{
	TimeoutPropose:          1500 * time.Millisecond,
	TimeoutProposeDelta:     500 * time.Millisecond,
	TimeoutPrevote:          1000 * time.Millisecond,
	TimeoutPrevoteDelta:     500 * time.Millisecond,
	TimeoutPrecommit:        1500 * time.Millisecond,
	TimeoutPrecommitDelta:   500 * time.Millisecond,
	TimeoutCommit:           800 * time.Millisecond,
	DelayedPrecommitTimeout: 0,
}

// GetConsensusTimeouts returns the consensus timeouts for appVersion.
func GetConsensusTimeouts(appVersion uint64) ConsensusTimeouts {
	if appVersion <= 10 {
		return v10ConsensusTimeouts
	}
	return v11ConsensusTimeouts
}
