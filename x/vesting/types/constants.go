package types

import warmage "github.com/petri-labs/warmage/types"

const (
	StakingRewardVestingName = "staking_reward_vesting"
	CommunityPoolVestingName = "community_pool_vesting"
	TeamVestingName          = "team_vesting"

	// Strate reserve pool controlled by governance.
	// Not used now, maybe future.
	StrategicReservePoolName = "strategic_reserve_pool"

	StakingRewardVestingTime = warmage.SecondsPer4Years
	CommunityPoolVestingTime = warmage.SecondsPer4Years
	TeamVestingTime          = warmage.SecondsPer4Years

	ClaimVestedPeriod = 10
)
