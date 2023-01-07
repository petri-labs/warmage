package types

import (
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	warmage "github.com/petri-labs/warmage/types"
)

// DefaultGenesis gets the raw genesis raw message for testing
func DefaultGenesis() *stakingtypes.GenesisState {
	params := stakingtypes.DefaultParams()
	params.BondDenom = warmage.BaseDenom
	return &stakingtypes.GenesisState{
		Params: params,
	}
}
