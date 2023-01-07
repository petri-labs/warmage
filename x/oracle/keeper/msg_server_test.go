package keeper_test

import (
	"context"
	"testing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	keepertest "github.com/petri-labs/warmage/testutil/keeper"
	"github.com/petri-labs/warmage/x/oracle/keeper"
	"github.com/petri-labs/warmage/x/oracle/types"
)

func setupMsgServer(t testing.TB) (types.MsgServer, context.Context) {
	k, ctx := keepertest.OracleKeeper(t)
	return keeper.NewMsgServerImpl(*k), sdk.WrapSDKContext(ctx)
}
