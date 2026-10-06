# GoCryptoTrader package Hyperliquid

<img src="../../common/gctlogo.png" alt="GoCryptoTrader logo" width="350px" height="350px" hspace="70">

[![Build Status](https://github.com/thrasher-corp/gocryptotrader/actions/workflows/tests.yml/badge.svg?branch=master)](https://github.com/thrasher-corp/gocryptotrader/actions/workflows/tests.yml)
[![Software License](https://img.shields.io/badge/License-MIT-orange.svg?style=flat-square)](https://github.com/thrasher-corp/gocryptotrader/blob/master/LICENSE)
[![GoDoc](https://godoc.org/github.com/thrasher-corp/gocryptotrader?status.svg)](https://godoc.org/github.com/thrasher-corp/gocryptotrader/exchanges/hyperliquid)
[![Coverage Status](https://codecov.io/gh/thrasher-corp/gocryptotrader/graph/badge.svg?token=41784B23TS)](https://codecov.io/gh/thrasher-corp/gocryptotrader)

This hyperliquid package is part of the GoCryptoTrader codebase.

## This is still in active development

You can track ideas, planned features and what's in progress on our [GoCryptoTrader Kanban board](https://github.com/orgs/thrasher-corp/projects/3).

Join our slack to discuss all things related to GoCryptoTrader! [GoCryptoTrader Slack](https://join.slack.com/t/gocryptotrader/shared_invite/zt-38z8abs3l-gH8AAOk8XND6DP5NfCiG_g)

## Hyperliquid Exchange

### Current Features

+ Spot, perpetual and builder-deployed (HIP-3) perpetual DEX markets
+ REST and websocket tickers, L2 orderbooks, trades and candles
+ Account balances, orders, fills, funding payments and ledger history
+ Vault, staking, referral, borrow/lend, outcome market and deployment information
+ Signed orders, cancels, modifications, TWAP and trailing stop orders, scheduled cancels, and leverage and margin
  updates
+ Transfers between spot, perpetual DEXs and accounts, USDC bridge withdrawals, staking, and API wallet and builder fee
  approvals
+ Funding rates, open interest and fee estimates

### How to enable

+ [Enable via configuration](../../config/README.md#enable-exchange-via-config-example)

### Credentials

Hyperliquid accounts are EVM addresses, so the credentials hold addresses and private keys rather than API keys:

+ `Key` is the account address, whose data is queried
+ `Secret` is optional, and is the private key of the account or of one of its approved API wallets
+ `SubAccount` is optional, and is a subaccount or vault the account controls, which then acts and is queried in its
  place

Account data and account websocket feeds need only `Key`. Signed actions need `Secret`, and an approved API wallet is
recommended for trading. Transfers, withdrawals and approvals are signed by the account itself, so they require
`Secret` to be the account's own private key. Before its first signed action, and after any rejected action, the
account, signer and subaccount or vault are checked against Hyperliquid's records.

Signed actions use millisecond nonces, which only increase within one GoCryptoTrader process, so the same signer must
not be used by several processes at once.

### Testnet

Set `useSandbox` to `true` to sign for testnet. Endpoints left at their production defaults then move to testnet, while
custom endpoints are kept, so `useSandbox` must match the environment behind a custom endpoint.

### Orders

+ Market orders require a `SlippageTolerance` above zero and below one, and are sent as IOC limit orders bounded by it.
+ Trigger orders must be reduce-only perpetual orders. Hyperliquid triggers on the mark price, so trigger orders and
  take-profit or stop-loss children must set `TriggerPriceType` to `order.MarkPrice`.
+ `ModifyOrder` replaces resting limit orders only. Hyperliquid replaces an order only when the replacement rests, so a
  GTC replacement that would match is rejected and trigger and IOC replacements are not supported.
+ Fee estimates use the account's rates when credentials are set, and the base tier rates otherwise.
+ Hyperliquid lists the 100 newest open orders of each DEX with their order details, so `GetActiveOrders` adds any older
  orders without their order type, time in force or trigger price.

### Balances

Unified and portfolio margin accounts hold one balance across spot and perpetuals, which is stored under spot. Other
accounts hold a balance per perpetual DEX, in that DEX's collateral token, stored under a subaccount ID of the account
address and DEX name.

### Websocket

The standard channels use these Hyperliquid feeds: tickers `activeAssetCtx`, orderbooks `l2Book`, trades `trades`,
candles `candle`, account orders `orderUpdates` and account trades `userEvents`. Any other feed, such as `bbo`,
`allMids` or `webData3`, is subscribed by using its name as the channel. Market feeds take one pair, account feeds must
be authenticated, and other feeds take no asset. Subscription parameters set the feed options:

+ `dex` selects a builder-deployed perpetual DEX for `allMids`, `clearinghouseState`, `openOrders` and `twapStates`
+ `nSigFigs` and `mantissa` aggregate an orderbook, and 5 levels select the fast orderbook, which Hyperliquid pushes
  about every half second instead of every 5 seconds
+ `aggregateByTime` combines an order's partial fills on `userFills`, and `ignorePortfolioMargin` omits portfolio margin
  amounts from `spotState`

Hyperliquid closes the connection, dropping every feed on it, when a subscription fails validation, so coins, DEXs and
options are validated first. Subscriptions complete when Hyperliquid acknowledges them, and are rolled back when it
rejects them or the connection drops. The recent trades Hyperliquid replays when trades are subscribed are not relayed.
Account trades include TWAP slice fills, and fills made while disconnected are not recovered from the websocket.

`WsPostInfo` sends an info request over the websocket instead of REST.

### Withdrawals

`WithdrawCryptocurrencyFunds` withdraws USDC through the Arbitrum bridge (Arbitrum Sepolia on testnet), whose fee
Hyperliquid deducts from the amount. Setting `InternalTransfer` sends USDC to another Hyperliquid account instead.
Deposits are credited to the address that sends them to the bridge, so there are no deposit addresses.

### Not supported

Token, market and outcome deployment, validator actions and the other platform administration actions.

### How to do REST public/private calls

When enabled through configuration, Hyperliquid is available through the `exchange.IBotExchange` interface:

```go
var h exchange.IBotExchange

for i := range bot.Exchanges {
	if bot.Exchanges[i].GetName() == "Hyperliquid" {
		h = bot.Exchanges[i]
	}
}

tick, err := h.UpdateTicker(ctx, pair, asset.Spot)
if err != nil {
	// Handle error
}

book, err := h.UpdateOrderbook(ctx, pair, asset.PerpetualContract)
if err != nil {
	// Handle error
}

trades, err := h.GetRecentTrades(ctx, pair, asset.PerpetualContract)
if err != nil {
	// Handle error
}
```

## Donations

<img src="../../docs/assets/donate.png" alt="Donate to GoCryptoTrader" hspace="70">

If this framework helped you in any way, or you would like to support the developers working on it, please donate Bitcoin to:

`bc1qk0jareu4jytc0cfrhr5wgshsq8282awpavfahc`
