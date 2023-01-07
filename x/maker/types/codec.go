package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	cdctypes "github.com/cosmos/cosmos-sdk/codec/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"

	// this line is used by starport scaffolding # 1
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func RegisterCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgMintBySwap{}, "warmage/MsgMintBySwap", nil)
	cdc.RegisterConcrete(&MsgBurnBySwap{}, "warmage/MsgBurnBySwap", nil)
	cdc.RegisterConcrete(&MsgBuyBacking{}, "warmage/MsgBuyBacking", nil)
	cdc.RegisterConcrete(&MsgSellBacking{}, "warmage/MsgSellBacking", nil)
	cdc.RegisterConcrete(&MsgMintByCollateral{}, "warmage/MsgMintByCollateral", nil)
	cdc.RegisterConcrete(&MsgBurnByCollateral{}, "warmage/MsgBurnByCollateral", nil)
	cdc.RegisterConcrete(&MsgDepositCollateral{}, "warmage/MsgDepositCollateral", nil)
	cdc.RegisterConcrete(&MsgRedeemCollateral{}, "warmage/MsgRedeemCollateral", nil)
	cdc.RegisterConcrete(&MsgLiquidateCollateral{}, "warmage/MsgLiquidateCollateral", nil)
}

func RegisterInterfaces(registry cdctypes.InterfaceRegistry) {
	registry.RegisterImplementations(
		(*govtypes.Content)(nil),
		&RegisterBackingProposal{},
		&RegisterCollateralProposal{},
		&SetBackingRiskParamsProposal{},
		&SetCollateralRiskParamsProposal{},
		&BatchSetBackingRiskParamsProposal{},
		&BatchSetCollateralRiskParamsProposal{},
	)

	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}

var (
	Amino     = codec.NewLegacyAmino()
	ModuleCdc = codec.NewAminoCodec(Amino)
)

func init() {
	RegisterCodec(Amino)
}
