package gauge_test

import (
	"testing"

	keepertest "github.com/petri-labs/warmage/testutil/keeper"
	"github.com/petri-labs/warmage/testutil/nullify"
	"github.com/petri-labs/warmage/x/gauge"
	"github.com/petri-labs/warmage/x/gauge/types"
	"github.com/stretchr/testify/require"
)

func TestGenesis(t *testing.T) {
	genesisState := types.GenesisState{
		Params: types.DefaultParams(),

		// this line is used by starport scaffolding # genesis/test/state
	}

	k, ctx := keepertest.GaugeKeeper(t)
	gauge.InitGenesis(ctx, *k, genesisState)
	got := gauge.ExportGenesis(ctx, *k)
	require.NotNil(t, got)

	nullify.Fill(&genesisState)
	nullify.Fill(got)

	// this line is used by starport scaffolding # genesis/test/assert
}
