package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	warmage "github.com/petri-labs/warmage/types"
	"github.com/petri-labs/warmage/x/maker/types"
	oracletypes "github.com/petri-labs/warmage/x/oracle/types"
)

type msgServer struct {
	Keeper
}

// NewMsgServerImpl returns an implementation of the MsgServer interface
// for the provided Keeper.
func NewMsgServerImpl(keeper Keeper) types.MsgServer {
	return &msgServer{Keeper: keeper}
}

var _ types.MsgServer = msgServer{}

func (m msgServer) MintBySwap(c context.Context, msg *types.MsgMintBySwap) (*types.MsgMintBySwapResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	sender, receiver, err := getSenderReceiver(msg.Sender, msg.To)
	if err != nil {
		return nil, err
	}

	backingIn, mageOut, mintOut, mintFee, err := m.Keeper.calculateMintBySwapOut(ctx, msg.BackingInMax, msg.MageInMax, msg.FullBacking)
	if err != nil {
		return nil, err
	}
	mintTotal := mintOut.Add(mintFee)

	if mintOut.IsLT(msg.MintOutMin) {
		return nil, sdkerrors.Wrapf(types.ErrOverSlippage, "mint out: %s", mintOut)
	}

	totalBacking, poolBacking, err := m.Keeper.getBacking(ctx, msg.BackingInMax.Denom)
	if err != nil {
		return nil, err
	}

	poolBacking.WarMinted = poolBacking.WarMinted.Add(mintTotal)
	poolBacking.Backing = poolBacking.Backing.Add(backingIn)
	poolBacking.MageBurned = poolBacking.MageBurned.Add(mageOut)

	totalBacking.WarMinted = totalBacking.WarMinted.Add(mintTotal)
	totalBacking.MageBurned = totalBacking.MageBurned.Add(mageOut)

	m.Keeper.SetPoolBacking(ctx, poolBacking)
	m.Keeper.SetTotalBacking(ctx, totalBacking)

	// take backing and mage coin
	err = m.Keeper.bankKeeper.SendCoinsFromAccountToModule(ctx, sender, types.ModuleName, sdk.NewCoins(backingIn, mageOut))
	if err != nil {
		return nil, err
	}
	// burn mage
	if mageOut.IsPositive() {
		err = m.Keeper.bankKeeper.BurnCoins(ctx, types.ModuleName, sdk.NewCoins(mageOut))
		if err != nil {
			return nil, err
		}
	}

	// mint war stablecoin
	err = m.Keeper.bankKeeper.MintCoins(ctx, types.ModuleName, sdk.NewCoins(mintTotal))
	if err != nil {
		return nil, err
	}
	// send war to receiver
	err = m.Keeper.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, receiver, sdk.NewCoins(mintOut))
	if err != nil {
		return nil, err
	}
	// send war fee to oracle
	if mintFee.IsPositive() {
		err = m.Keeper.bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, oracletypes.ModuleName, sdk.NewCoins(mintFee))
		if err != nil {
			return nil, err
		}
	}

	ctx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(types.EventTypeMintBySwap,
			sdk.NewAttribute(types.AttributeKeyCoinIn, sdk.NewCoins(backingIn, mageOut).String()),
			sdk.NewAttribute(types.AttributeKeyCoinOut, mintOut.String()),
			sdk.NewAttribute(types.AttributeKeyFee, mintFee.String()),
		),
		sdk.NewEvent(
			sdk.EventTypeMessage,
			sdk.NewAttribute(sdk.AttributeKeyModule, types.AttributeValueCategory),
		),
	})

	return &types.MsgMintBySwapResponse{
		BackingIn: backingIn,
		MageIn:    mageOut,
		MintOut:   mintOut,
		MintFee:   mintFee,
	}, nil
}

func (m msgServer) BurnBySwap(c context.Context, msg *types.MsgBurnBySwap) (*types.MsgBurnBySwapResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	sender, receiver, err := getSenderReceiver(msg.Sender, msg.To)
	if err != nil {
		return nil, err
	}

	backingOut, mageOut, burnFee, err := m.Keeper.calculateBurnBySwapOut(ctx, msg.BurnIn, msg.BackingOutMin.Denom)
	if err != nil {
		return nil, err
	}
	burnActual := msg.BurnIn.Sub(burnFee)

	if backingOut.IsLT(msg.BackingOutMin) {
		return nil, sdkerrors.Wrapf(types.ErrOverSlippage, "backing out: %s", backingOut)
	}
	if mageOut.IsLT(msg.MageOutMin) {
		return nil, sdkerrors.Wrapf(types.ErrOverSlippage, "mage out: %s", mageOut)
	}

	totalBacking, poolBacking, err := m.Keeper.getBacking(ctx, msg.BackingOutMin.Denom)
	if err != nil {
		return nil, err
	}

	poolBacking.Backing = poolBacking.Backing.Sub(backingOut)
	// allow MageBurned to be negative which means minted mage
	// here use Int.Sub() to bypass Coin.Sub() negativeness check
	poolBacking.MageBurned.Amount = poolBacking.MageBurned.Amount.Sub(mageOut.Amount)
	totalBacking.MageBurned.Amount = totalBacking.MageBurned.Amount.Sub(mageOut.Amount)
	// allow WarMinted to be negative which means burned war
	poolBacking.WarMinted.Amount = poolBacking.WarMinted.Amount.Sub(burnActual.Amount)
	totalBacking.WarMinted.Amount = totalBacking.WarMinted.Amount.Sub(burnActual.Amount)

	m.Keeper.SetPoolBacking(ctx, poolBacking)
	m.Keeper.SetTotalBacking(ctx, totalBacking)

	// take war stablecoin
	err = m.Keeper.bankKeeper.SendCoinsFromAccountToModule(ctx, sender, types.ModuleName, sdk.NewCoins(msg.BurnIn))
	if err != nil {
		return nil, err
	}
	// burn war
	err = m.Keeper.bankKeeper.BurnCoins(ctx, types.ModuleName, sdk.NewCoins(burnActual))
	if err != nil {
		return nil, err
	}
	// send war fee to oracle
	err = m.Keeper.bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, oracletypes.ModuleName, sdk.NewCoins(burnFee))
	if err != nil {
		return nil, err
	}

	// mint mage
	err = m.Keeper.bankKeeper.MintCoins(ctx, types.ModuleName, sdk.NewCoins(mageOut))
	if err != nil {
		return nil, err
	}
	// send backing and mage to receiver
	err = m.Keeper.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, receiver, sdk.NewCoins(backingOut, mageOut))
	if err != nil {
		return nil, err
	}

	ctx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(types.EventTypeBurnBySwap,
			sdk.NewAttribute(types.AttributeKeyCoinIn, msg.BurnIn.String()),
			sdk.NewAttribute(types.AttributeKeyCoinOut, sdk.NewCoins(backingOut, mageOut).String()),
			sdk.NewAttribute(types.AttributeKeyFee, burnFee.String()),
		),
		sdk.NewEvent(
			sdk.EventTypeMessage,
			sdk.NewAttribute(sdk.AttributeKeyModule, types.AttributeValueCategory),
		),
	})

	return &types.MsgBurnBySwapResponse{
		BurnFee:    burnFee,
		BackingOut: backingOut,
		MageOut:    mageOut,
	}, nil
}

func (m msgServer) BuyBacking(c context.Context, msg *types.MsgBuyBacking) (*types.MsgBuyBackingResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	sender, receiver, err := getSenderReceiver(msg.Sender, msg.To)
	if err != nil {
		return nil, err
	}

	backingOut, buybackFee, err := m.Keeper.calculateBuyBackingOut(ctx, msg.MageIn, msg.BackingOutMin.Denom)
	if err != nil {
		return nil, err
	}

	if backingOut.IsLT(msg.BackingOutMin) {
		return nil, sdkerrors.Wrapf(types.ErrOverSlippage, "backing out: %s", backingOut)
	}

	totalBacking, poolBacking, err := m.Keeper.getBacking(ctx, msg.BackingOutMin.Denom)
	if err != nil {
		return nil, err
	}

	poolBacking.Backing = poolBacking.Backing.Sub(backingOut).Sub(buybackFee)
	poolBacking.MageBurned = poolBacking.MageBurned.Add(msg.MageIn)
	totalBacking.MageBurned = totalBacking.MageBurned.Add(msg.MageIn)

	m.Keeper.SetPoolBacking(ctx, poolBacking)
	m.Keeper.SetTotalBacking(ctx, totalBacking)

	// take mage-in
	err = m.Keeper.bankKeeper.SendCoinsFromAccountToModule(ctx, sender, types.ModuleName, sdk.NewCoins(msg.MageIn))
	if err != nil {
		return nil, err
	}
	// burn mage
	err = m.Keeper.bankKeeper.BurnCoins(ctx, types.ModuleName, sdk.NewCoins(msg.MageIn))
	if err != nil {
		return nil, err
	}

	// send backing to receiver
	err = m.Keeper.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, receiver, sdk.NewCoins(backingOut))
	if err != nil {
		return nil, err
	}
	// send fee to oracle
	err = m.Keeper.bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, oracletypes.ModuleName, sdk.NewCoins(buybackFee))
	if err != nil {
		return nil, err
	}

	ctx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(types.EventTypeBuyBacking,
			sdk.NewAttribute(types.AttributeKeyCoinIn, msg.MageIn.String()),
			sdk.NewAttribute(types.AttributeKeyCoinOut, backingOut.String()),
			sdk.NewAttribute(types.AttributeKeyFee, buybackFee.String()),
		),
		sdk.NewEvent(
			sdk.EventTypeMessage,
			sdk.NewAttribute(sdk.AttributeKeyModule, types.AttributeValueCategory),
		),
	})

	return &types.MsgBuyBackingResponse{
		BackingOut: backingOut,
		BuybackFee: buybackFee,
	}, nil
}

func (m msgServer) SellBacking(c context.Context, msg *types.MsgSellBacking) (*types.MsgSellBackingResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	sender, receiver, err := getSenderReceiver(msg.Sender, msg.To)
	if err != nil {
		return nil, err
	}

	mageOut, rebackFee, err := m.Keeper.calculateSellBackingOut(ctx, msg.BackingIn)
	if err != nil {
		return nil, err
	}
	mageMInt := mageOut.Add(rebackFee)

	if mageOut.IsLT(msg.MageOutMin) {
		return nil, sdkerrors.Wrapf(types.ErrOverSlippage, "mage out: %s", mageOut)
	}

	totalBacking, poolBacking, err := m.Keeper.getBacking(ctx, msg.BackingIn.Denom)
	if err != nil {
		return nil, err
	}

	poolBacking.Backing = poolBacking.Backing.Add(msg.BackingIn)

	// allow MageBurned to be negative which means minted mage
	// here use Int.Sub() to bypass Coin.Sub() negativeness check
	poolBacking.MageBurned.Amount = poolBacking.MageBurned.Amount.Sub(mageMInt.Amount)
	totalBacking.MageBurned.Amount = totalBacking.MageBurned.Amount.Sub(mageMInt.Amount)

	m.Keeper.SetPoolBacking(ctx, poolBacking)
	m.Keeper.SetTotalBacking(ctx, totalBacking)

	// take backing-in
	err = m.Keeper.bankKeeper.SendCoinsFromAccountToModule(ctx, sender, types.ModuleName, sdk.NewCoins(msg.BackingIn))
	if err != nil {
		return nil, err
	}

	// mint mage
	err = m.Keeper.bankKeeper.MintCoins(ctx, types.ModuleName, sdk.NewCoins(mageMInt))
	if err != nil {
		return nil, err
	}
	// send mage to receiver
	err = m.Keeper.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, receiver, sdk.NewCoins(mageOut))
	if err != nil {
		return nil, err
	}
	// send fee to oracle
	err = m.Keeper.bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, oracletypes.ModuleName, sdk.NewCoins(rebackFee))
	if err != nil {
		return nil, err
	}

	ctx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(types.EventTypeSellBacking,
			sdk.NewAttribute(types.AttributeKeyCoinIn, msg.BackingIn.String()),
			sdk.NewAttribute(types.AttributeKeyCoinOut, mageOut.String()),
			sdk.NewAttribute(types.AttributeKeyFee, rebackFee.String()),
		),
		sdk.NewEvent(
			sdk.EventTypeMessage,
			sdk.NewAttribute(sdk.AttributeKeyModule, types.AttributeValueCategory),
		),
	})

	return &types.MsgSellBackingResponse{
		MageOut:   mageOut,
		RebackFee: rebackFee,
	}, nil
}

func (m msgServer) MintByCollateral(c context.Context, msg *types.MsgMintByCollateral) (*types.MsgMintByCollateralResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	sender, receiver, err := getSenderReceiver(msg.Sender, msg.To)
	if err != nil {
		return nil, err
	}

	mintFee, totalColl, poolColl, accColl, err := m.Keeper.calculateMintByCollateral(ctx, sender, msg.CollateralDenom, msg.MintOut)
	if err != nil {
		return nil, err
	}
	mintTotal := msg.MintOut.Add(mintFee)

	m.Keeper.SetAccountCollateral(ctx, sender, accColl)
	m.Keeper.SetPoolCollateral(ctx, poolColl)
	m.Keeper.SetTotalCollateral(ctx, totalColl)

	// mint war
	err = m.Keeper.bankKeeper.MintCoins(ctx, types.ModuleName, sdk.NewCoins(mintTotal))
	if err != nil {
		return nil, err
	}
	// send war to receiver
	err = m.Keeper.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, receiver, sdk.NewCoins(msg.MintOut))
	if err != nil {
		return nil, err
	}
	// send mint fee to oracle
	if mintFee.IsPositive() {
		err = m.Keeper.bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, oracletypes.ModuleName, sdk.NewCoins(mintFee))
		if err != nil {
			return nil, err
		}
	}

	ctx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(types.EventTypeMintByCollateral,
			sdk.NewAttribute(types.AttributeKeyCoinOut, msg.MintOut.String()),
			sdk.NewAttribute(types.AttributeKeyFee, mintFee.String()),
		),
		sdk.NewEvent(
			sdk.EventTypeMessage,
			sdk.NewAttribute(sdk.AttributeKeyModule, types.AttributeValueCategory),
		),
	})

	return &types.MsgMintByCollateralResponse{
		MintFee: mintFee,
	}, nil
}

func (m msgServer) BurnByCollateral(c context.Context, msg *types.MsgBurnByCollateral) (*types.MsgBurnByCollateralResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)
	sender, _, err := getSenderReceiver(msg.Sender, "")
	if err != nil {
		return nil, err
	}

	collateralDenom := msg.CollateralDenom

	collateralParams, err := m.Keeper.getAvailableCollateralParams(ctx, collateralDenom)
	if err != nil {
		return nil, err
	}

	totalColl, poolColl, accColl, err := m.Keeper.getCollateral(ctx, sender, collateralDenom)
	if err != nil {
		return nil, err
	}

	settleInterestFee(ctx, &accColl, &poolColl, &totalColl, *collateralParams.InterestFee)

	// compute burn-in, repay interest first
	if !accColl.WarDebt.IsPositive() {
		return nil, sdkerrors.Wrapf(types.ErrAccountNoDebt, "account has no debt for %s collateral", collateralDenom)
	}
	repayIn := sdk.NewCoin(msg.RepayInMax.Denom, sdk.MinInt(accColl.WarDebt.Amount, msg.RepayInMax.Amount))
	repayInterest := sdk.NewCoin(msg.RepayInMax.Denom, sdk.MinInt(accColl.LastInterest.Amount, repayIn.Amount))
	burn := repayIn.Sub(repayInterest)

	// update debt
	accColl.LastInterest = accColl.LastInterest.Sub(repayInterest)
	accColl.WarDebt = accColl.WarDebt.Sub(repayIn)
	poolColl.WarDebt = poolColl.WarDebt.Sub(repayIn)
	totalColl.WarDebt = totalColl.WarDebt.Sub(repayIn)

	// eventually update collateral
	m.Keeper.SetAccountCollateral(ctx, sender, accColl)
	m.Keeper.SetPoolCollateral(ctx, poolColl)
	m.Keeper.SetTotalCollateral(ctx, totalColl)

	// take war
	err = m.Keeper.bankKeeper.SendCoinsFromAccountToModule(ctx, sender, types.ModuleName, sdk.NewCoins(repayIn))
	if err != nil {
		return nil, err
	}
	// burn war
	if burn.IsPositive() {
		err = m.Keeper.bankKeeper.BurnCoins(ctx, types.ModuleName, sdk.NewCoins(burn))
		if err != nil {
			return nil, err
		}
	}
	// send fee to oracle
	if repayInterest.IsPositive() {
		err = m.Keeper.bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, oracletypes.ModuleName, sdk.NewCoins(repayInterest))
		if err != nil {
			return nil, err
		}
	}

	ctx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(types.EventTypeBurnByCollateral,
			sdk.NewAttribute(types.AttributeKeyCoinIn, repayIn.String()),
			sdk.NewAttribute(types.AttributeKeyFee, repayInterest.String()),
		),
		sdk.NewEvent(
			sdk.EventTypeMessage,
			sdk.NewAttribute(sdk.AttributeKeyModule, types.AttributeValueCategory),
		),
	})

	return &types.MsgBurnByCollateralResponse{
		RepayIn: repayIn,
	}, nil
}

func (m msgServer) DepositCollateral(c context.Context, msg *types.MsgDepositCollateral) (*types.MsgDepositCollateralResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)

	collateralDenom := msg.CollateralIn.Denom

	sender, receiver, err := getSenderReceiver(msg.Sender, msg.To)
	if err != nil {
		return nil, err
	}

	collateralParams, err := m.Keeper.getAvailableCollateralParams(ctx, collateralDenom)
	if err != nil {
		return nil, err
	}

	totalColl, poolColl, accColl, err := m.Keeper.getCollateral(ctx, receiver, collateralDenom, true)
	if err != nil {
		return nil, err
	}

	settleInterestFee(ctx, &accColl, &poolColl, &totalColl, *collateralParams.InterestFee)

	accColl.Collateral = accColl.Collateral.Add(msg.CollateralIn)
	poolColl.Collateral = poolColl.Collateral.Add(msg.CollateralIn)
	accColl.MageCollateralized = accColl.MageCollateralized.Add(msg.MageIn)
	poolColl.MageCollateralized = poolColl.MageCollateralized.Add(msg.MageIn)
	totalColl.MageCollateralized = totalColl.MageCollateralized.Add(msg.MageIn)

	if collateralParams.MaxCollateral != nil && poolColl.Collateral.Amount.GT(*collateralParams.MaxCollateral) {
		return nil, sdkerrors.Wrap(types.ErrCollateralCeiling, "")
	}

	m.Keeper.SetAccountCollateral(ctx, receiver, accColl)
	m.Keeper.SetPoolCollateral(ctx, poolColl)
	m.Keeper.SetTotalCollateral(ctx, totalColl)

	// take collateral from sender
	err = m.Keeper.bankKeeper.SendCoinsFromAccountToModule(ctx, sender, types.ModuleName, sdk.NewCoins(msg.CollateralIn, msg.MageIn))
	if err != nil {
		return nil, err
	}

	ctx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(types.EventTypeDepositCollateral,
			sdk.NewAttribute(types.AttributeKeyCoinIn, sdk.NewCoins(msg.CollateralIn, msg.MageIn).String()),
		),
		sdk.NewEvent(
			sdk.EventTypeMessage,
			sdk.NewAttribute(sdk.AttributeKeyModule, types.AttributeValueCategory),
		),
	})

	return &types.MsgDepositCollateralResponse{}, nil
}

func (m msgServer) RedeemCollateral(c context.Context, msg *types.MsgRedeemCollateral) (*types.MsgRedeemCollateralResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)

	collateralDenom := msg.CollateralOut.Denom

	sender, receiver, err := getSenderReceiver(msg.Sender, msg.To)
	if err != nil {
		return nil, err
	}

	collateralParams, err := m.Keeper.getAvailableCollateralParams(ctx, collateralDenom)
	if err != nil {
		return nil, err
	}

	totalColl, poolColl, accColl, err := m.Keeper.getCollateral(ctx, sender, collateralDenom)
	if err != nil {
		return nil, err
	}

	settleInterestFee(ctx, &accColl, &poolColl, &totalColl, *collateralParams.InterestFee)

	// update collateral
	accColl.Collateral = accColl.Collateral.Sub(msg.CollateralOut)
	poolColl.Collateral = poolColl.Collateral.Sub(msg.CollateralOut)
	accColl.MageCollateralized = accColl.MageCollateralized.Sub(msg.MageOut)
	poolColl.MageCollateralized = poolColl.MageCollateralized.Sub(msg.MageOut)
	totalColl.MageCollateralized = totalColl.MageCollateralized.Sub(msg.MageOut)

	_, maxDebtInUSD, err := m.Keeper.maxLoanToValueForAccount(ctx, &accColl, &collateralParams)
	if err != nil {
		return nil, err
	}

	if accColl.WarDebt.Amount.ToDec().Mul(warmage.MicroUSWTarget).GT(maxDebtInUSD) {
		return nil, sdkerrors.Wrapf(types.ErrAccountInsufficientCollateral, "account collateral insufficient: %s", collateralDenom)
	}

	// eventually persist collateral
	m.Keeper.SetAccountCollateral(ctx, sender, accColl)
	m.Keeper.SetPoolCollateral(ctx, poolColl)
	m.Keeper.SetTotalCollateral(ctx, totalColl)

	// send collateral to receiver
	err = m.Keeper.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, receiver, sdk.NewCoins(msg.CollateralOut, msg.MageOut))
	if err != nil {
		return nil, err
	}

	ctx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(types.EventTypeRedeemCollateral,
			sdk.NewAttribute(types.AttributeKeyCoinOut, sdk.NewCoins(msg.CollateralOut, msg.MageOut).String()),
		),
		sdk.NewEvent(
			sdk.EventTypeMessage,
			sdk.NewAttribute(sdk.AttributeKeyModule, types.AttributeValueCategory),
		),
	})

	return &types.MsgRedeemCollateralResponse{}, nil
}

func (m msgServer) LiquidateCollateral(c context.Context, msg *types.MsgLiquidateCollateral) (*types.MsgLiquidateCollateralResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)

	collateralDenom := msg.Collateral.Denom

	sender, receiver, err := getSenderReceiver(msg.Sender, msg.To)
	if err != nil {
		return nil, err
	}
	debtor, err := sdk.AccAddressFromBech32(msg.Debtor)
	if err != nil {
		return nil, err
	}

	collateralParams, err := m.Keeper.getAvailableCollateralParams(ctx, collateralDenom)
	if err != nil {
		return nil, err
	}

	totalColl, poolColl, accColl, err := m.Keeper.getCollateral(ctx, debtor, collateralDenom)
	if err != nil {
		return nil, err
	}

	settleInterestFee(ctx, &accColl, &poolColl, &totalColl, *collateralParams.InterestFee)

	// get prices in usd
	collateralPrice, err := m.Keeper.oracleKeeper.GetExchangeRate(ctx, collateralDenom)
	if err != nil {
		return nil, err
	}

	// check whether undercollateralized
	liquidationValue := accColl.Collateral.Amount.ToDec().Mul(collateralPrice).Mul(*collateralParams.LiquidationThreshold)
	if accColl.WarDebt.Amount.ToDec().Mul(warmage.MicroUSWTarget).LT(liquidationValue) {
		return nil, sdkerrors.Wrap(types.ErrNotUndercollateralized, "")
	}

	if msg.Collateral.Amount.GT(accColl.Collateral.Amount) {
		return nil, sdkerrors.Wrap(types.ErrCollateralCoinInsufficient, "")
	}

	liquidationFee := msg.Collateral.Amount.ToDec().Mul(*collateralParams.LiquidationFee)
	commissionFee := sdk.NewCoin(collateralDenom, liquidationFee.Mul(m.Keeper.LiquidationCommissionFee(ctx)).TruncateInt())
	collateralOut := msg.Collateral.Sub(commissionFee)
	repayIn := sdk.NewCoin(warmage.MicroUSWDenom, msg.Collateral.Amount.ToDec().Sub(liquidationFee).Mul(collateralPrice).Quo(warmage.MicroUSWTarget).TruncateInt())

	if msg.RepayInMax.IsLT(repayIn) {
		return nil, sdkerrors.Wrap(types.ErrMerSlippage, "")
	}

	// repay for debtor as much as possible, and repay interest first
	repayDebt := sdk.NewCoin(warmage.MicroUSWDenom, sdk.MinInt(accColl.WarDebt.Amount, repayIn.Amount))
	warRefund := repayIn.Sub(repayDebt)

	repayInterest := sdk.NewCoin(warmage.MicroUSWDenom, sdk.MinInt(accColl.LastInterest.Amount, repayDebt.Amount))
	accColl.LastInterest = accColl.LastInterest.Sub(repayInterest)

	accColl.WarDebt = accColl.WarDebt.Sub(repayDebt)
	poolColl.WarDebt = poolColl.WarDebt.Sub(repayDebt)
	totalColl.WarDebt = totalColl.WarDebt.Sub(repayDebt)
	accColl.Collateral = accColl.Collateral.Sub(msg.Collateral)
	poolColl.Collateral = poolColl.Collateral.Sub(msg.Collateral)

	// eventually persist collateral
	m.Keeper.SetAccountCollateral(ctx, debtor, accColl)
	m.Keeper.SetPoolCollateral(ctx, poolColl)
	m.Keeper.SetTotalCollateral(ctx, totalColl)

	// take war from sender
	err = m.Keeper.bankKeeper.SendCoinsFromAccountToModule(ctx, sender, types.ModuleName, sdk.NewCoins(repayIn))
	if err != nil {
		return nil, err
	}
	// burn war debt
	err = m.Keeper.bankKeeper.BurnCoins(ctx, types.ModuleName, sdk.NewCoins(repayDebt))
	if err != nil {
		return nil, err
	}
	// send excess war to debtor
	if warRefund.IsPositive() {
		err = m.Keeper.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, debtor, sdk.NewCoins(warRefund))
		if err != nil {
			return nil, err
		}
	}

	// send collateral to receiver
	err = m.Keeper.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, receiver, sdk.NewCoins(collateralOut))
	if err != nil {
		return nil, err
	}
	// send liquidation commission fee to oracle
	err = m.Keeper.bankKeeper.SendCoinsFromModuleToModule(ctx, types.ModuleName, oracletypes.ModuleName, sdk.NewCoins(commissionFee))
	if err != nil {
		return nil, err
	}

	ctx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(types.EventTypeLiquidateCollateral,
			sdk.NewAttribute(types.AttributeKeyCoinIn, repayIn.String()),
			sdk.NewAttribute(types.AttributeKeyCoinOut, collateralOut.String()),
			sdk.NewAttribute(types.AttributeKeyFee, commissionFee.String()),
		),
		sdk.NewEvent(
			sdk.EventTypeMessage,
			sdk.NewAttribute(sdk.AttributeKeyModule, types.AttributeValueCategory),
		),
	})

	return &types.MsgLiquidateCollateralResponse{
		RepayIn:       repayIn,
		CollateralOut: collateralOut,
	}, nil
}

func (k Keeper) getBacking(ctx sdk.Context, denom string) (total types.TotalBacking, pool types.PoolBacking, err error) {
	total, found := k.GetTotalBacking(ctx)
	if !found {
		err = sdkerrors.Wrapf(types.ErrBackingCoinNotFound, "backing coin denomination not found: %s", denom)
		return
	}
	pool, found = k.GetPoolBacking(ctx, denom)
	if !found {
		err = sdkerrors.Wrapf(types.ErrBackingCoinNotFound, "backing coin denomination not found: %s", denom)
		return
	}
	return
}

func (k Keeper) getCollateral(ctx sdk.Context, account sdk.AccAddress, denom string, allowNewAccount ...bool) (total types.TotalCollateral, pool types.PoolCollateral, acc types.AccountCollateral, err error) {
	total, found := k.GetTotalCollateral(ctx)
	if !found {
		err = sdkerrors.Wrapf(types.ErrCollateralCoinNotFound, "collateral coin denomination not found: %s", denom)
		return
	}
	pool, found = k.GetPoolCollateral(ctx, denom)
	if !found {
		err = sdkerrors.Wrapf(types.ErrCollateralCoinNotFound, "collateral coin denomination not found: %s", denom)
		return
	}
	acc, found = k.GetAccountCollateral(ctx, account, denom)
	if !found {
		if len(allowNewAccount) > 0 && allowNewAccount[0] {
			acc = types.AccountCollateral{
				Account:             account.String(),
				Collateral:          sdk.NewCoin(denom, sdk.ZeroInt()),
				WarDebt:             sdk.NewCoin(warmage.MicroUSWDenom, sdk.ZeroInt()),
				MageCollateralized:  sdk.NewCoin(warmage.AttoMageDenom, sdk.ZeroInt()),
				LastInterest:        sdk.NewCoin(warmage.MicroUSWDenom, sdk.ZeroInt()),
				LastSettlementBlock: ctx.BlockHeight(),
			}
		} else {
			err = sdkerrors.Wrapf(types.ErrAccountNoCollateral, "account has no collateral: %s", denom)
			return
		}
	}
	return
}

func settleInterestFee(ctx sdk.Context, acc *types.AccountCollateral, pool *types.PoolCollateral, total *types.TotalCollateral, apr sdk.Dec) {
	period := ctx.BlockHeight() - acc.LastSettlementBlock
	if period == 0 {
		// short circuit
		return
	}

	// principal debt, excluding interest debt
	principalDebt := acc.WarDebt.Sub(acc.LastInterest)
	interestOfPeriod := principalDebt.Amount.ToDec().Mul(apr).MulInt64(period).QuoInt64(int64(warmage.BlocksPerYear)).RoundInt()

	// update remaining interest accumulation
	acc.LastInterest = acc.LastInterest.AddAmount(interestOfPeriod)
	// update debt
	acc.WarDebt = acc.WarDebt.AddAmount(interestOfPeriod)
	pool.WarDebt = pool.WarDebt.AddAmount(interestOfPeriod)
	total.WarDebt = total.WarDebt.AddAmount(interestOfPeriod)
	// update settlement block
	acc.LastSettlementBlock = ctx.BlockHeight()
}

func (k Keeper) maxLoanToValueForAccount(ctx sdk.Context, acc *types.AccountCollateral, collateralParams *types.CollateralRiskParams) (availableLTV, maxDebtInUSD sdk.Dec, err error) {
	collateralPrice, err := k.oracleKeeper.GetExchangeRate(ctx, acc.Collateral.Denom)
	if err != nil {
		return
	}
	magePrice, err := k.oracleKeeper.GetExchangeRate(ctx, warmage.AttoMageDenom)
	if err != nil {
		return
	}

	collateralInUSD := acc.Collateral.Amount.ToDec().Mul(collateralPrice)
	collateralizedMageInUSD := acc.MageCollateralized.Amount.ToDec().Mul(magePrice)
	if !collateralInUSD.IsPositive() {
		return sdk.ZeroDec(), sdk.ZeroDec(), nil
	}

	catalyticRatio := sdk.MinDec(collateralizedMageInUSD.Quo(collateralInUSD), *collateralParams.CatalyticMageRatio)
	// actualCatalyticRatio / maxCatalyticRatio = (availableLTV - basicLTV) / (maxLTV - basicLTV)
	availableLTV = collateralParams.LoanToValue.Sub(*collateralParams.BasicLoanToValue).Mul(catalyticRatio).Quo(*collateralParams.CatalyticMageRatio).Add(*collateralParams.BasicLoanToValue)
	maxDebtInUSD = collateralInUSD.Mul(availableLTV)

	return
}

func getSenderReceiver(senderStr, toStr string) (sender sdk.AccAddress, receiver sdk.AccAddress, err error) {
	sender, err = sdk.AccAddressFromBech32(senderStr)
	if err != nil {
		return
	}
	receiver = sender
	if len(toStr) > 0 {
		// user specifies receiver
		receiver, err = sdk.AccAddressFromBech32(toStr)
		if err != nil {
			return
		}
	}
	return
}
