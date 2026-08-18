# GoCryptoTrader package Kraken

<img src="/common/gctlogo.png?raw=true" width="350px" height="350px" hspace="70">


[![Build Status](https://github.com/thrasher-corp/gocryptotrader/actions/workflows/tests.yml/badge.svg?branch=master)](https://github.com/thrasher-corp/gocryptotrader/actions/workflows/tests.yml)
[![Software License](https://img.shields.io/badge/License-MIT-orange.svg?style=flat-square)](https://github.com/thrasher-corp/gocryptotrader/blob/master/LICENSE)
[![GoDoc](https://godoc.org/github.com/thrasher-corp/gocryptotrader?status.svg)](https://godoc.org/github.com/thrasher-corp/gocryptotrader/exchanges/kraken)
[![Coverage Status](https://codecov.io/gh/thrasher-corp/gocryptotrader/graph/badge.svg?token=41784B23TS)](https://codecov.io/gh/thrasher-corp/gocryptotrader)


This kraken package is part of the GoCryptoTrader codebase.

## This is still in active development

You can track ideas, planned features and what's in progress on our [GoCryptoTrader Kanban board](https://github.com/orgs/thrasher-corp/projects/3).

Join our slack to discuss all things related to GoCryptoTrader! [GoCryptoTrader Slack](https://join.slack.com/t/gocryptotrader/shared_invite/zt-38z8abs3l-gH8AAOk8XND6DP5NfCiG_g)

## Kraken Exchange

### Current Features

+ REST Support

### How to enable

+ [Enable via configuration](https://github.com/thrasher-corp/gocryptotrader/tree/master/config#enable-exchange-via-config-example)

+ Individual package example below:

```go
	// Exchanges will be abstracted out in further updates and examples will be
	// supplied then
```

### How to do REST public/private calls

+ If enabled via "configuration".json file the exchange will be added to the
IBotExchange array in the ```go var bot Bot``` and you will only be able to use
the wrapper interface functions for accessing exchange data. View routines.go
for an example of integration usage with GoCryptoTrader. Rudimentary example
below:

main.go
```go
var k exchange.IBotExchange

for i := range bot.Exchanges {
	if bot.Exchanges[i].GetName() == "Kraken" {
		k = bot.Exchanges[i]
	}
}

// Public calls - wrapper functions

// Fetches current ticker information
tick, err := k.UpdateTicker(...)
if err != nil {
	// Handle error
}

// Fetches current orderbook information
ob, err := k.UpdateOrderbook(...)
if err != nil {
	// Handle error
}

// Private calls - wrapper functions - make sure your APIKEY and APISECRET are
// set and AuthenticatedAPISupport is set to true

// Fetches current account information
accountInfo, err := k.GetAccountInfo()
if err != nil {
	// Handle error
}
```

+ If enabled via individually importing package, rudimentary example below:

```go
// Public calls

// Fetches current ticker information
ticker, err := k.GetTicker()
if err != nil {
	// Handle error
}

// Fetches current orderbook information
ob, err := k.GetOrderBook()
if err != nil {
	// Handle error
}

// Private calls - make sure your APIKEY and APISECRET are set and
// AuthenticatedAPISupport is set to true

// GetUserInfo returns account info
accountInfo, err := k.GetUserInfo(...)
if err != nil {
	// Handle error
}

// Submits an order to the exchange and returns its tradeID
tradeID, err := k.Trade(...)
if err != nil {
	// Handle error
}
```

## Spot REST live tests

Spot endpoint tests use deterministic per-endpoint mocks by default. Add the
`mock_test_off` build tag to run each endpoint test's final `live` subtest:

```sh
go test -tags=mock_test_off -run '^TestGetSystemStatus$' ./exchanges/kraken
```

Public endpoints need no credentials. Authenticated endpoints require test API
credentials in the `apiCredentials` value in `kraken_test.go`; keep that local
configuration uncommitted and never commit credentials.
Account-specific read-only tests skip unless their required environment values
are set:

- `GCT_KRAKEN_SPOT_LIVE_ORDER_ID`
- `GCT_KRAKEN_SPOT_LIVE_TRADE_ID`
- `GCT_KRAKEN_SPOT_LIVE_LEDGER_ID`
- `GCT_KRAKEN_SPOT_LIVE_EXPORT_ID`
- `GCT_KRAKEN_SPOT_LIVE_EARN_STRATEGY_ID`
- `GCT_KRAKEN_SPOT_LIVE_WITHDRAWAL_ASSET`
- `GCT_KRAKEN_SPOT_LIVE_WITHDRAWAL_KEY`
- `GCT_KRAKEN_SPOT_LIVE_WITHDRAWAL_INFO_AMOUNT`

Every state-changing endpoint has its own `false` opt-in in `kraken_test.go`.
Run these tests individually after reviewing the exact payload; some operations
have no safe automatic rollback.

| Endpoint test | Opt-in | Required environment values |
| --- | --- | --- |
| `TestAmendOrder` | `canAmendRealSpotOrder` | `GCT_KRAKEN_SPOT_LIVE_AMEND_ORDER_ID`, `GCT_KRAKEN_SPOT_LIVE_AMEND_PRICE` |
| `TestCancelAllOpenOrders` | `canCancelAllRealSpotOrders` | None; cancels every open Spot order |
| `TestCancelAllOrdersAfter` | `canArmRealSpotDeadMansSwitch` | None; the test disables the switch after the live call |
| `TestAddOrderBatch` | `canValidateRealSpotOrderBatch` | None; requests use Kraken's validation-only mode |
| `TestCancelOrderBatch` | `canCancelRealSpotOrderBatch` | `GCT_KRAKEN_SPOT_LIVE_BATCH_CANCEL_ORDER_ID` |
| `TestAddOrder` | `canValidateRealSpotOrder` | None; the request uses Kraken's validation-only mode |
| `TestCancelExistingOrder` | `canCancelRealSpotOrder` | `GCT_KRAKEN_SPOT_LIVE_CANCEL_ORDER_ID` |
| `TestWithdrawFunds` | `canWithdrawRealSpotFunds` | `GCT_KRAKEN_SPOT_LIVE_WITHDRAWAL_ASSET`, `GCT_KRAKEN_SPOT_LIVE_WITHDRAWAL_KEY`, `GCT_KRAKEN_SPOT_LIVE_WITHDRAWAL_AMOUNT` |
| `TestCancelWithdrawal` | `canCancelRealSpotWithdrawal` | `GCT_KRAKEN_SPOT_LIVE_WITHDRAWAL_REFERENCE_ASSET`, `GCT_KRAKEN_SPOT_LIVE_WITHDRAWAL_REFERENCE_ID` |
| `TestWalletTransfer` | `canTransferRealSpotWalletFunds` | `GCT_KRAKEN_SPOT_LIVE_WALLET_TRANSFER_ASSET`, `GCT_KRAKEN_SPOT_LIVE_WALLET_TRANSFER_AMOUNT` |
| `TestAllocateEarnFunds` | `canAllocateRealSpotEarnFunds` | `GCT_KRAKEN_SPOT_LIVE_EARN_STRATEGY_ID`, `GCT_KRAKEN_SPOT_LIVE_EARN_ALLOCATE_AMOUNT` |
| `TestDeallocateEarnFunds` | `canDeallocateRealSpotEarnFunds` | `GCT_KRAKEN_SPOT_LIVE_EARN_STRATEGY_ID`, `GCT_KRAKEN_SPOT_LIVE_EARN_DEALLOCATE_AMOUNT` |
| `TestRequestExportReport` | `canRequestRealSpotExportReport` | None; the test cancels the requested export |
| `TestDeleteExportReport` | `canDeleteRealSpotExportReport` | `GCT_KRAKEN_SPOT_LIVE_DELETE_EXPORT_ID` |
| `TestCreateSubaccount` | `canCreateRealSpotSubaccount` | `GCT_KRAKEN_SPOT_LIVE_SUBACCOUNT_USERNAME`, `GCT_KRAKEN_SPOT_LIVE_SUBACCOUNT_EMAIL` |
| `TestAccountTransfer` | `canTransferRealSpotSubaccountFunds` | `GCT_KRAKEN_SPOT_LIVE_ACCOUNT_TRANSFER_ASSET`, `GCT_KRAKEN_SPOT_LIVE_ACCOUNT_TRANSFER_AMOUNT`, `GCT_KRAKEN_SPOT_LIVE_ACCOUNT_TRANSFER_FROM`, `GCT_KRAKEN_SPOT_LIVE_ACCOUNT_TRANSFER_TO` |

Set `GCT_SKIP_LIVE_TESTS=true` when checking that the tagged package compiles
without running live tests.

## Donations

<img src="/docs/assets/donate.png" hspace="70">

If this framework helped you in any way, or you would like to support the developers working on it, please donate Bitcoin to:

***bc1qk0jareu4jytc0cfrhr5wgshsq8282awpavfahc***
