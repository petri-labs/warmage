---
order: 5
---

# Tokens

Learn about the different types of native tokens/coins available in Warmage. {synopsis}

## Native Coins

The two native coin types of Warmage are **War** and **Mage**:

- **War**: Stablecoins that track the price of fiat currencies, and they are named for their fiat counterparts. In the
  early stage of the mainnet launch, it will mainly issue **WarUSD**, or **USW**, which tracks/pegs the price of $USD.
- **Mage**: Native staking coin that partially absorbs the price volatility of War. Users stake Mage to validators to
  add blocks of transactions to the blockchain, and earn various fees and rewards. Holders of Mage also can vote on
  proposals and participate in on-chain governance. And Mage is also used for gas consumption for running smart
  contracts on the EVM.

Warmage uses [Atto](https://en.wikipedia.org/wiki/Atto-) Mage or `amage` as the base denomination to maintain parity
with Ethereum.

```
1 mage = 1 * 1e18 amage
```

And the base denomination of War is `uusd`.

```
1 WarUSD = 1 USW = 1 * 1e6 uusd
```

## Other Cosmos Coins

Accounts can own Cosmos SDK coins in their balance, which are used for operations with other Cosmos modules and
transactions. Example of these are IBC vouchers.

## ERC-20 Tokens

Any ERC-20 tokens are natively supported by the EVM of Warmage.
