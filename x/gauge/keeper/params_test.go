package keeper_test

import (
	"testing"

	testkeeper "github.com/petri-labs/warmage/testutil/keeper"
	"github.com/petri-labs/warmage/x/gauge/types"
	"github.com/stretchr/testify/require"
)

func TestGetParams(t *testing.T) {
	k, ctx := testkeeper.GaugeKeeper(t)
	params := types.DefaultParams()

	k.SetParams(ctx, params)

	require.EqualValues(t, params, k.GetParams(ctx))
}
