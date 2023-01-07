package keeper

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	warmage "github.com/petri-labs/warmage/types"
	"github.com/petri-labs/warmage/x/maker/types"
)

func (k Keeper) calculateMintBySwapIn(
	ctx sdk.Context,
	mintOut sdk.Coin,
	backingDenom string,
	fullBacking bool,
) (
	backingIn sdk.Coin,
	mageOut sdk.Coin,
	mintFee sdk.Coin,
	err error,
) {
	backingIn = sdk.NewCoin(backingDenom, sdk.ZeroInt())
	mageOut = sdk.NewCoin(warmage.AttoMageDenom, sdk.ZeroInt())
	mintFee = sdk.NewCoin(warmage.MicroUSWDenom, sdk.ZeroInt())

	err = k.checkMintPriceLowerBound(ctx)
	if err != nil {
		return
	}

	backingParams, err := k.getAvailableBackingParams(ctx, backingDenom)
	if err != nil {
		return
	}

	// get prices in usd
	backingPrice, err := k.oracleKeeper.GetExchangeRate(ctx, backingDenom)
	if err != nil {
		return
	}
	magePrice, err := k.oracleKeeper.GetExchangeRate(ctx, warmage.AttoMageDenom)
	if err != nil {
		return
	}

	mintFee = computeFee(mintOut, backingParams.MintFee)
	mintTotal := mintOut.Add(mintFee)
	mintTotalInUSD := mintTotal.Amount.ToDec().Mul(warmage.MicroUSWTarget)

	_, poolBacking, err := k.getBacking(ctx, backingDenom)
	if err != nil {
		return
	}
	poolBacking.WarMinted = poolBacking.WarMinted.Add(mintTotal)
	if backingParams.MaxWarMint != nil && poolBacking.WarMinted.Amount.GT(*backingParams.MaxWarMint) {
		err = sdkerrors.Wrapf(types.ErrWarCeiling, "war over ceiling")
		return
	}

	backingRatio := k.GetBackingRatio(ctx)
	if backingRatio.GTE(sdk.OneDec()) || fullBacking {
		// full/over backing, or user selects full backing
		backingIn.Amount = mintTotalInUSD.QuoRoundUp(backingPrice).RoundInt()
	} else if backingRatio.IsZero() {
		// full algorithmic
		mageOut.Amount = mintTotalInUSD.QuoRoundUp(magePrice).RoundInt()
	} else {
		// fractional
		backingIn.Amount = mintTotalInUSD.Mul(backingRatio).QuoRoundUp(backingPrice).RoundInt()
		mageOut.Amount = mintTotalInUSD.Mul(sdk.OneDec().Sub(backingRatio)).QuoRoundUp(magePrice).RoundInt()
	}

	poolBacking.Backing = poolBacking.Backing.Add(backingIn)
	if backingParams.MaxBacking != nil && poolBacking.Backing.Amount.GT(*backingParams.MaxBacking) {
		err = sdkerrors.Wrapf(types.ErrBackingCeiling, "backing over ceiling")
		return
	}

	return
}

func (k Keeper) calculateMintBySwapOut(
	ctx sdk.Context,
	backingInMax sdk.Coin,
	mageInMax sdk.Coin,
	fullBacking bool,
) (
	backingIn sdk.Coin,
	mageOut sdk.Coin,
	mintOut sdk.Coin,
	mintFee sdk.Coin,
	err error,
) {
	backingDenom := backingInMax.Denom

	err = k.checkMintPriceLowerBound(ctx)
	if err != nil {
		return
	}

	backingParams, err := k.getAvailableBackingParams(ctx, backingDenom)
	if err != nil {
		return
	}

	// get prices in uusd
	backingPrice, err := k.oracleKeeper.GetExchangeRate(ctx, backingDenom)
	if err != nil {
		return
	}
	magePrice, err := k.oracleKeeper.GetExchangeRate(ctx, warmage.AttoMageDenom)
	if err != nil {
		return
	}

	backingRatio := k.GetBackingRatio(ctx)

	backingInMaxInUSD := backingPrice.MulInt(backingInMax.Amount)
	mageInMaxInUSD := magePrice.MulInt(mageInMax.Amount)

	mintTotalInUSD := sdk.ZeroDec()
	backingIn = sdk.NewCoin(backingDenom, sdk.ZeroInt())
	mageOut = sdk.NewCoin(warmage.AttoMageDenom, sdk.ZeroInt())

	if backingRatio.GTE(sdk.OneDec()) || fullBacking {
		// full/over backing, or user selects full backing
		mintTotalInUSD = backingInMaxInUSD
		backingIn.Amount = backingInMax.Amount
	} else if backingRatio.IsZero() {
		// full algorithmic
		mintTotalInUSD = mageInMaxInUSD
		mageOut.Amount = mageInMax.Amount
	} else {
		// fractional
		max1 := backingInMaxInUSD.Quo(backingRatio)
		max2 := mageInMaxInUSD.Quo(sdk.OneDec().Sub(backingRatio))
		if backingInMax.IsPositive() && (mageInMax.IsZero() || max1.LTE(max2)) {
			mintTotalInUSD = max1
			backingIn.Amount = backingInMax.Amount
			mageOut.Amount = mintTotalInUSD.Mul(sdk.OneDec().Sub(backingRatio)).QuoRoundUp(magePrice).RoundInt()
			if mageInMax.IsPositive() && mageInMax.IsLT(mageOut) {
				mageOut.Amount = mageInMax.Amount
			}
		} else {
			mintTotalInUSD = max2
			mageOut.Amount = mageInMax.Amount
			backingIn.Amount = mintTotalInUSD.Mul(backingRatio).QuoRoundUp(backingPrice).RoundInt()
			if backingInMax.IsPositive() && backingInMax.IsLT(backingIn) {
				backingIn.Amount = backingInMax.Amount
			}
		}
	}

	mintTotal := sdk.NewCoin(warmage.MicroUSWDenom, mintTotalInUSD.Quo(warmage.MicroUSWTarget).TruncateInt())

	_, poolBacking, err := k.getBacking(ctx, backingDenom)
	if err != nil {
		return
	}

	poolBacking.WarMinted = poolBacking.WarMinted.AddAmount(mintTotal.Amount)
	if backingParams.MaxWarMint != nil && poolBacking.WarMinted.Amount.GT(*backingParams.MaxWarMint) {
		err = sdkerrors.Wrap(types.ErrWarCeiling, "")
		return
	}

	poolBacking.Backing = poolBacking.Backing.Add(backingIn)
	if backingParams.MaxBacking != nil && poolBacking.Backing.Amount.GT(*backingParams.MaxBacking) {
		err = sdkerrors.Wrap(types.ErrBackingCeiling, "")
		return
	}

	mintFee = computeFee(mintTotal, backingParams.MintFee)
	mintOut = mintTotal.Sub(mintFee)
	return
}

func (k Keeper) calculateBurnBySwapIn(
	ctx sdk.Context,
	backingOutMax sdk.Coin,
	mageOutMax sdk.Coin,
) (
	burnIn sdk.Coin,
	backingOut sdk.Coin,
	mageOut sdk.Coin,
	burnFee sdk.Coin,
	err error,
) {
	backingDenom := backingOutMax.Denom

	burnIn = sdk.NewCoin(warmage.MicroUSWDenom, sdk.ZeroInt())
	backingOut = sdk.NewCoin(backingOutMax.Denom, sdk.ZeroInt())
	mageOut = sdk.NewCoin(warmage.AttoMageDenom, sdk.ZeroInt())
	burnFee = sdk.NewCoin(warmage.MicroUSWDenom, sdk.ZeroInt())

	err = k.checkBurnPriceUpperBound(ctx)
	if err != nil {
		return
	}

	backingParams, err := k.getAvailableBackingParams(ctx, backingDenom)
	if err != nil {
		return
	}

	// get prices in usd
	backingPrice, err := k.oracleKeeper.GetExchangeRate(ctx, backingDenom)
	if err != nil {
		return
	}
	magePrice, err := k.oracleKeeper.GetExchangeRate(ctx, warmage.AttoMageDenom)
	if err != nil {
		return
	}

	backingOutMaxInUSD := backingPrice.MulInt(backingOutMax.Amount)
	mageOutMaxInUSD := magePrice.MulInt(mageOutMax.Amount)

	burnActualInUSD := sdk.ZeroDec()
	backingRatio := k.GetBackingRatio(ctx)
	if backingRatio.GTE(sdk.OneDec()) {
		// full/over backing
		burnActualInUSD = backingOutMaxInUSD
		backingOut.Amount = backingOutMax.Amount
	} else if backingRatio.IsZero() {
		// full algorithmic
		burnActualInUSD = mageOutMaxInUSD
		mageOut.Amount = mageOutMax.Amount
	} else {
		// fractional
		burnActualWithBackingInUSD := backingOutMaxInUSD.Quo(backingRatio)
		burnActualWithMageInUSD := mageOutMaxInUSD.Quo(sdk.OneDec().Sub(backingRatio))
		if mageOutMax.IsZero() || (backingOutMax.IsPositive() && burnActualWithBackingInUSD.LT(burnActualWithMageInUSD)) {
			burnActualInUSD = burnActualWithBackingInUSD
			backingOut.Amount = backingOutMax.Amount
			mageOut.Amount = burnActualInUSD.Mul(sdk.OneDec().Sub(backingRatio)).QuoRoundUp(magePrice).RoundInt()
		} else {
			burnActualInUSD = burnActualWithMageInUSD
			mageOut.Amount = mageOutMax.Amount
			backingOut.Amount = burnActualInUSD.Mul(backingRatio).QuoRoundUp(backingPrice).RoundInt()
		}
	}

	moduleOwnedBacking := k.bankKeeper.GetBalance(ctx, k.accountKeeper.GetModuleAddress(types.ModuleName), backingDenom)
	if moduleOwnedBacking.IsLT(backingOut) {
		err = sdkerrors.Wrapf(types.ErrBackingCoinInsufficient, "backing coin out(%s) < balance(%s)", backingOut, moduleOwnedBacking)
		return
	}

	burnFeeRate := sdk.ZeroDec()
	if backingParams.BurnFee != nil {
		burnFeeRate = *backingParams.BurnFee
	}

	burnInValue := burnActualInUSD.Quo(warmage.MicroUSWTarget).Quo(sdk.OneDec().Sub(burnFeeRate))
	burnFeeValue := burnInValue.Mul(burnFeeRate)
	burnIn = sdk.NewCoin(warmage.MicroUSWDenom, burnInValue.RoundInt())
	burnFee = sdk.NewCoin(warmage.MicroUSWDenom, burnFeeValue.RoundInt())
	return
}

func (k Keeper) calculateBurnBySwapOut(
	ctx sdk.Context,
	burnIn sdk.Coin,
	backingDenom string,
) (
	backingOut sdk.Coin,
	mageOut sdk.Coin,
	burnFee sdk.Coin,
	err error,
) {
	err = k.checkBurnPriceUpperBound(ctx)
	if err != nil {
		return
	}

	backingParams, err := k.getAvailableBackingParams(ctx, backingDenom)
	if err != nil {
		return
	}

	// get prices in usd
	backingPrice, err := k.oracleKeeper.GetExchangeRate(ctx, backingDenom)
	if err != nil {
		return
	}
	magePrice, err := k.oracleKeeper.GetExchangeRate(ctx, warmage.AttoMageDenom)
	if err != nil {
		return
	}

	backingRatio := k.GetBackingRatio(ctx)

	burnFee = computeFee(burnIn, backingParams.BurnFee)
	burnActual := burnIn.Sub(burnFee)
	burnActualInUSD := burnActual.Amount.ToDec().Mul(warmage.MicroUSWTarget)

	backingOut = sdk.NewCoin(backingDenom, sdk.ZeroInt())
	mageOut = sdk.NewCoin(warmage.AttoMageDenom, sdk.ZeroInt())

	if backingRatio.GTE(sdk.OneDec()) {
		// full/over backing
		backingOut.Amount = burnActualInUSD.QuoTruncate(backingPrice).TruncateInt()
	} else if backingRatio.IsZero() {
		// full algorithmic
		mageOut.Amount = burnActualInUSD.QuoTruncate(magePrice).TruncateInt()
	} else {
		// fractional
		backingOut.Amount = burnActualInUSD.Mul(backingRatio).QuoTruncate(backingPrice).TruncateInt()
		mageOut.Amount = burnActualInUSD.Mul(sdk.OneDec().Sub(backingRatio)).QuoTruncate(magePrice).TruncateInt()
	}

	_, poolBacking, err := k.getBacking(ctx, backingDenom)
	if err != nil {
		return
	}
	moduleOwnedBacking := k.bankKeeper.GetBalance(ctx, k.accountKeeper.GetModuleAddress(types.ModuleName), backingDenom)

	poolBackingBalance := sdk.NewCoin(backingDenom, sdk.MinInt(poolBacking.Backing.Amount, moduleOwnedBacking.Amount))
	if poolBackingBalance.IsLT(backingOut) {
		err = sdkerrors.Wrapf(types.ErrBackingCoinInsufficient, "backing coin out(%s) > balance(%s)", backingOut, poolBackingBalance)
		return
	}

	return
}

func (k Keeper) calculateBuyBackingIn(
	ctx sdk.Context,
	backingOut sdk.Coin,
) (
	mageOut sdk.Coin,
	buybackFee sdk.Coin,
	err error,
) {
	backingDenom := backingOut.Denom

	backingParams, err := k.getAvailableBackingParams(ctx, backingDenom)
	if err != nil {
		return
	}

	// get prices in usd
	backingPrice, err := k.oracleKeeper.GetExchangeRate(ctx, backingDenom)
	if err != nil {
		return
	}
	magePrice, err := k.oracleKeeper.GetExchangeRate(ctx, warmage.AttoMageDenom)
	if err != nil {
		return
	}

	excessBackingValue, err := k.getExcessBackingValue(ctx)
	if err != nil {
		return
	}

	backingOutTotal := sdk.NewCoin(backingDenom, backingOut.Amount.ToDec().Quo(sdk.OneDec().Sub(*backingParams.BuybackFee)).TruncateInt())
	mageOutValue := backingOutTotal.Amount.ToDec().Mul(backingPrice)

	if mageOutValue.GT(excessBackingValue.ToDec()) {
		err = sdkerrors.Wrap(types.ErrBackingCoinInsufficient, "")
		return
	}

	_, poolBacking, err := k.getBacking(ctx, backingDenom)
	if err != nil {
		return
	}
	moduleOwnedBacking := k.bankKeeper.GetBalance(ctx, k.accountKeeper.GetModuleAddress(types.ModuleName), backingDenom)

	poolBackingBalance := sdk.NewCoin(backingDenom, sdk.MinInt(poolBacking.Backing.Amount, moduleOwnedBacking.Amount))
	if poolBackingBalance.IsLT(backingOutTotal) {
		err = sdkerrors.Wrapf(types.ErrBackingCoinInsufficient, "backing coin out(%s) > balance(%s)", backingOutTotal, poolBackingBalance)
		return
	}

	mageOut = sdk.NewCoin(warmage.AttoMageDenom, mageOutValue.Quo(magePrice).RoundInt())
	buybackFee = sdk.NewCoin(backingDenom, backingOutTotal.Amount.ToDec().Mul(*backingParams.BuybackFee).RoundInt())
	return
}

func (k Keeper) calculateBuyBackingOut(
	ctx sdk.Context,
	mageOut sdk.Coin,
	backingDenom string,
) (
	backingOut sdk.Coin,
	buybackFee sdk.Coin,
	err error,
) {
	backingParams, err := k.getAvailableBackingParams(ctx, backingDenom)
	if err != nil {
		return
	}

	// get prices in usd
	backingPrice, err := k.oracleKeeper.GetExchangeRate(ctx, backingDenom)
	if err != nil {
		return
	}
	magePrice, err := k.oracleKeeper.GetExchangeRate(ctx, warmage.AttoMageDenom)
	if err != nil {
		return
	}

	excessBackingValue, err := k.getExcessBackingValue(ctx)
	if err != nil {
		return
	}

	mageOutValue := mageOut.Amount.ToDec().Mul(magePrice)
	if mageOutValue.GT(excessBackingValue.ToDec()) {
		err = sdkerrors.Wrap(types.ErrBackingCoinInsufficient, "")
		return
	}

	backingOutTotal := sdk.NewCoin(backingDenom, mageOutValue.Quo(backingPrice).TruncateInt())

	_, poolBacking, err := k.getBacking(ctx, backingDenom)
	if err != nil {
		return
	}
	moduleOwnedBacking := k.bankKeeper.GetBalance(ctx, k.accountKeeper.GetModuleAddress(types.ModuleName), backingDenom)

	poolBackingBalance := sdk.NewCoin(backingDenom, sdk.MinInt(poolBacking.Backing.Amount, moduleOwnedBacking.Amount))
	if poolBackingBalance.IsLT(backingOutTotal) {
		err = sdkerrors.Wrapf(types.ErrBackingCoinInsufficient, "backing coin out(%s) > balance(%s)", backingOutTotal, poolBackingBalance)
		return
	}

	buybackFee = computeFee(backingOutTotal, backingParams.BuybackFee)
	backingOut = backingOutTotal.Sub(buybackFee)
	return
}

func (k Keeper) calculateSellBackingIn(
	ctx sdk.Context,
	mageOut sdk.Coin,
	backingDenom string,
) (
	backingIn sdk.Coin,
	rebackFee sdk.Coin,
	err error,
) {
	backingParams, err := k.getAvailableBackingParams(ctx, backingDenom)
	if err != nil {
		return
	}

	// get prices in usd
	backingPrice, err := k.oracleKeeper.GetExchangeRate(ctx, backingDenom)
	if err != nil {
		return
	}
	magePrice, err := k.oracleKeeper.GetExchangeRate(ctx, warmage.AttoMageDenom)
	if err != nil {
		return
	}

	_, poolBacking, err := k.getBacking(ctx, backingDenom)
	if err != nil {
		return
	}

	excessBackingValue, err := k.getExcessBackingValue(ctx)
	if err != nil {
		return
	}
	missingBackingValue := excessBackingValue.Neg()
	availableMageMint := missingBackingValue.ToDec().Quo(magePrice)

	bonusRatio := k.RebackBonus(ctx)

	mageMInt := mageOut.Amount.ToDec().Quo(sdk.OneDec().Add(bonusRatio).Sub(*backingParams.RebackFee))

	backingIn = sdk.NewCoin(backingDenom, mageMInt.Mul(magePrice).Quo(backingPrice).RoundInt())
	rebackFee = sdk.NewCoin(warmage.AttoMageDenom, mageMInt.Mul(*backingParams.RebackFee).RoundInt())

	poolBacking.Backing = poolBacking.Backing.Add(backingIn)
	if backingParams.MaxBacking != nil && poolBacking.Backing.Amount.GT(*backingParams.MaxBacking) {
		err = sdkerrors.Wrap(types.ErrBackingCeiling, "")
		return
	}
	if mageMInt.GT(availableMageMint) {
		err = sdkerrors.Wrap(types.ErrMageCoinInsufficient, "")
		return
	}

	return
}

func (k Keeper) calculateSellBackingOut(
	ctx sdk.Context,
	backingIn sdk.Coin,
) (
	mageOut sdk.Coin,
	rebackFee sdk.Coin,
	err error,
) {
	backingDenom := backingIn.Denom

	backingParams, err := k.getAvailableBackingParams(ctx, backingDenom)
	if err != nil {
		return
	}

	// get prices in usd
	backingPrice, err := k.oracleKeeper.GetExchangeRate(ctx, backingDenom)
	if err != nil {
		return
	}
	magePrice, err := k.oracleKeeper.GetExchangeRate(ctx, warmage.AttoMageDenom)
	if err != nil {
		return
	}

	_, poolBacking, err := k.getBacking(ctx, backingDenom)
	if err != nil {
		return
	}

	poolBacking.Backing = poolBacking.Backing.Add(backingIn)
	if backingParams.MaxBacking != nil && poolBacking.Backing.Amount.GT(*backingParams.MaxBacking) {
		err = sdkerrors.Wrap(types.ErrBackingCeiling, "")
		return
	}

	excessBackingValue, err := k.getExcessBackingValue(ctx)
	if err != nil {
		return
	}
	missingBackingValue := excessBackingValue.Neg()
	availableMageMint := missingBackingValue.ToDec().Quo(magePrice)

	bonusRatio := k.RebackBonus(ctx)
	mageMInt := sdk.NewCoin(warmage.AttoMageDenom, backingIn.Amount.ToDec().Mul(backingPrice).Quo(magePrice).TruncateInt())
	bonus := computeFee(mageMInt, &bonusRatio)
	rebackFee = computeFee(mageMInt, backingParams.RebackFee)

	if mageMInt.Amount.ToDec().GT(availableMageMint) {
		err = sdkerrors.Wrap(types.ErrMageCoinInsufficient, "")
		return
	}

	mageOut = mageMInt.Add(bonus).Sub(rebackFee)
	return
}

func (k Keeper) calculateMintByCollateral(
	ctx sdk.Context,
	account sdk.AccAddress,
	collateralDenom string,
	mintOut sdk.Coin,
) (
	mintFee sdk.Coin,
	totalColl types.TotalCollateral,
	poolColl types.PoolCollateral,
	accColl types.AccountCollateral,
	err error,
) {
	collateralParams, err := k.getAvailableCollateralParams(ctx, collateralDenom)
	if err != nil {
		return
	}

	// get prices in usd
	collateralPrice, err := k.oracleKeeper.GetExchangeRate(ctx, collateralDenom)
	if err != nil {
		return
	}
	magePrice, err := k.oracleKeeper.GetExchangeRate(ctx, warmage.AttoMageDenom)
	if err != nil {
		return
	}

	totalColl, poolColl, accColl, err = k.getCollateral(ctx, account, collateralDenom)
	if err != nil {
		return
	}

	// settle interest fee
	settleInterestFee(ctx, &accColl, &poolColl, &totalColl, *collateralParams.InterestFee)

	// compute mint total
	mintFee = computeFee(mintOut, collateralParams.MintFee)
	mintTotal := mintOut.Add(mintFee)

	// update war debt
	accColl.WarDebt = accColl.WarDebt.Add(mintTotal)
	poolColl.WarDebt = poolColl.WarDebt.Add(mintTotal)
	totalColl.WarDebt = totalColl.WarDebt.Add(mintTotal)

	if collateralParams.MaxWarMint != nil && poolColl.WarDebt.Amount.GT(*collateralParams.MaxWarMint) {
		err = sdkerrors.Wrapf(types.ErrWarCeiling, "")
		return
	}

	collateralValue := accColl.Collateral.Amount.ToDec().Mul(collateralPrice)
	mageCollateralizedValue := accColl.MageCollateralized.Amount.ToDec().Mul(magePrice)
	if !collateralValue.IsPositive() {
		err = sdkerrors.Wrapf(types.ErrAccountInsufficientCollateral, "")
		return
	}

	actualCatalyticRatio := sdk.MinDec(mageCollateralizedValue.Quo(collateralValue), *collateralParams.CatalyticMageRatio)

	// actualCatalyticRatio / catalyticRatio = (availableLTV - basicLTV) / (maxLTV - basicLTV)
	availableLTV := *collateralParams.BasicLoanToValue
	if collateralParams.CatalyticMageRatio.IsPositive() {
		availableLTV = availableLTV.Add(actualCatalyticRatio.Mul(collateralParams.LoanToValue.Sub(*collateralParams.BasicLoanToValue)).Quo(*collateralParams.CatalyticMageRatio))
	}
	availableDebtMax := collateralValue.Mul(availableLTV).Quo(warmage.MicroUSWTarget).TruncateInt()

	if availableDebtMax.LT(accColl.WarDebt.Amount) {
		err = sdkerrors.Wrapf(types.ErrAccountInsufficientCollateral, "")
		return
	}

	return
}

func computeFee(coin sdk.Coin, rate *sdk.Dec) sdk.Coin {
	amt := sdk.ZeroInt()
	if rate != nil {
		amt = coin.Amount.ToDec().Mul(*rate).RoundInt()
	}
	return sdk.NewCoin(coin.Denom, amt)
}

func (k Keeper) checkMintPriceLowerBound(ctx sdk.Context) error {
	warPrice, err := k.oracleKeeper.GetExchangeRate(ctx, warmage.MicroUSWDenom)
	if err != nil {
		return err
	}
	// market price must be >= target price + mint bias
	mintPriceLowerBound := warmage.MicroUSWTarget.Mul(sdk.OneDec().Add(k.MintPriceBias(ctx)))
	if warPrice.LT(mintPriceLowerBound) {
		return sdkerrors.Wrapf(types.ErrWarPriceTooLow, "%s price too low: %s", warmage.MicroUSWDenom, warPrice)
	}
	return nil
}

func (k Keeper) checkBurnPriceUpperBound(ctx sdk.Context) error {
	warPrice, err := k.oracleKeeper.GetExchangeRate(ctx, warmage.MicroUSWDenom)
	if err != nil {
		return err
	}
	// market price must be <= target price - burn bias
	burnPriceUpperBound := warmage.MicroUSWTarget.Mul(sdk.OneDec().Sub(k.BurnPriceBias(ctx)))
	if warPrice.GT(burnPriceUpperBound) {
		return sdkerrors.Wrapf(types.ErrWarPriceTooHigh, "%s price too high: %s", warmage.MicroUSWDenom, warPrice)
	}
	return nil
}

func (k Keeper) getAvailableBackingParams(ctx sdk.Context, backingDenom string) (backingParams types.BackingRiskParams, err error) {
	backingParams, found := k.GetBackingRiskParams(ctx, backingDenom)
	if !found {
		err = sdkerrors.Wrapf(types.ErrBackingCoinNotFound, "backing coin denomination not found: %s", backingDenom)
		return
	}
	if !backingParams.Enabled {
		err = sdkerrors.Wrapf(types.ErrBackingCoinDisabled, "backing coin disabled: %s", backingDenom)
		return
	}
	return
}

func (k Keeper) getAvailableCollateralParams(ctx sdk.Context, collateralDenom string) (collateralParams types.CollateralRiskParams, err error) {
	collateralParams, found := k.GetCollateralRiskParams(ctx, collateralDenom)
	if !found {
		err = sdkerrors.Wrapf(types.ErrCollateralCoinNotFound, "collateral coin denomination not found: %s", collateralDenom)
		return
	}
	if !collateralParams.Enabled {
		err = sdkerrors.Wrapf(types.ErrCollateralCoinDisabled, "collateral coin disabled: %s", collateralDenom)
		return
	}
	return
}

func (k Keeper) getExcessBackingValue(ctx sdk.Context) (excessBackingValue sdk.Int, err error) {
	totalBacking, found := k.GetTotalBacking(ctx)
	if !found {
		err = sdkerrors.Wrapf(types.ErrBackingCoinNotFound, "total backing not found")
		return
	}

	backingRatio := k.GetBackingRatio(ctx)
	requiredBackingValue := totalBacking.WarMinted.Amount.ToDec().Mul(backingRatio).Ceil().TruncateInt()
	if requiredBackingValue.IsNegative() {
		requiredBackingValue = sdk.ZeroInt()
	}

	totalBackingValue, err := k.totalBackingInUSD(ctx)
	if err != nil {
		return
	}

	// may be negative
	excessBackingValue = totalBackingValue.Sub(requiredBackingValue)
	return
}

func (k Keeper) totalBackingInUSD(ctx sdk.Context) (sdk.Int, error) {
	totalBackingValue := sdk.ZeroDec()
	for _, pool := range k.GetAllPoolBacking(ctx) {
		// get price in usd
		backingPrice, err := k.oracleKeeper.GetExchangeRate(ctx, pool.Backing.Denom)
		if err != nil {
			return sdk.Int{}, err
		}
		totalBackingValue = totalBackingValue.Add(pool.Backing.Amount.ToDec().Mul(backingPrice))
	}
	return totalBackingValue.TruncateInt(), nil
}
