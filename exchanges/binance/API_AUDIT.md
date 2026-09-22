# Binance API audit — 21 September 2026

This audit accompanies [PR 1546](https://github.com/thrasher-corp/gocryptotrader/pull/1546),
combined with master at `c6e47c6e1e6d6f7353148379d587188ec47c0367`.
The PR head is `8db72437445be70e746241a68052fc0b3cf30db3`.
Observed production responses take precedence over documentation, followed by
the [official SDK schemas](https://github.com/binance/binance-connector-python/tree/506e738ade8b53056db177a37189077de1004ff1).

## Scope and evidence

The audit covers the product families already represented in this integration:
Spot, Margin, USD-M and COIN-M Futures, Options, Portfolio Margin and Portfolio
Margin Pro, wallet, sub-accounts, loans, staking, earn, mining, convert, algo,
copy trading, fiat, gift cards, pay, rebate, C2C, Binance Link and OMS/Fast API.

| Evidence | Coverage |
| --- | --- |
| [REST inventory](testdata/api_audit/rest.json) | 690 implemented SDK routes and 59 additional documented routes |
| [WebSocket API inventory](testdata/api_audit/websocket.json) | 84 methods: 55 Spot, 28 Futures and the Margin listen-token subscription |
| [Stream inventory](testdata/api_audit/streams.json) | 113 event schemas, including nested fields |
| [Documented responses](testdata/documented_responses.json) | 661 fixtures exercised against 666 response-model cases |
| [Public REST manifest](testdata/public_api/manifest.json) | 114 live test cases and an Options discovery request |
| [Public stream manifest](testdata/public_api/market_streams_manifest.json) | Observed and unobserved subscriptions, capture times and response digests |
| [Legacy inventory](testdata/api_audit/legacy.json) | 57 Go calls classified separately by current evidence |

The inventories retain parameter names, documented types and requiredness,
recursive response field paths, implementation names and source links. Each
JSON record occupies one line so endpoint changes are straightforward to find.
Request field candidates include shared helpers; `extraParameterReview`
distinguishes parameters rejected before transmission, unused helper branches
and additional parameters accepted by production.
The [audit manifest](testdata/api_audit/manifest.json) pins sources, counts and
inventory digests. Union response fields include both success and error
alternatives; API error envelopes are decoded by the common transport.

Ten older SDK route versions are represented by their latest implemented
versions. Alpha, Stocks, stock-contract signing and Web3 Prediction are new
product families outside this integration's existing scope; their 61 SDK
routes are recorded as exclusions. The TradFi trading-session stream is also
excluded.

## Live verification

Public REST probes refuse credentials, signatures and non-GET requests.
`TestPublicEndpointsLive` checks response decoding and every observed field;
`TestPublicEndpointsRecorded` repeats those checks through the existing VCR
mock server. Public Spot and USD-M WebSocket API probes have corresponding
recorded replay tests. Production market frames cover Spot, Options and both
futures products. Live futures frames establish the `st` routing discriminator,
the placement of liquidation fields, and additional trade and mark-price fields.

Fixtures retain all object fields. Large arrays retain representative entries
for every distinct recursive field shape, with the exact transformation and
original response digest in their manifests. Empty live arrays establish the
request and empty-result shape; documented fixtures cover populated results.

Quiet channels are explicitly listed as unobserved in the stream manifest.
A successful subscription acknowledgement does not establish event-field
compatibility. Documented fixtures cover those events until production frames
are available.

Both futures recent-trade endpoints accepted the legacy `fromId` parameter,
and the trading-schedule endpoint accepted `symbol`. These observations preserve
request compatibility; they do not establish undocumented filtering behaviour.

No authenticated success request was made. Authenticated success tests remain
credential-gated. An offline documented response does not establish account
eligibility, permissions or production success.

## Offline verification

- The PR's `mocktester` and VCR infrastructure cover existing endpoint and
  sentinel-error tests and replay the captured public responses.
- `httptest.NewTestServer` serves documented responses and checks request paths,
  methods, parameters, JSON bodies, authentication and API error envelopes.
- GCT `mockws` exercises the registered WebSocket connections, signing,
  subscriptions, event dispatch, listen-key renewal, expiry and cancellation.
- `TestPrivateStreamKeepaliveTiming` uses virtual time to check the production
  30-minute renewal interval and cancellation without shortening that interval.
- Shared number, currency and time types accept the documented representations.
  Exact mixed string/number IDs use `types.PreciseNumber`; `types.Time` is the
  shared timestamp type. Binance's `Timestamp` additionally accepts documented
  `-1` sentinels and UTC date strings.

## Account and configuration behaviour

Margin token creation supports `symbol`, `isIsolated` and millisecond `validity`;
the subscription supports both fresh tokens and renewal tokens, subscription ID
zero and timestamp precision from the
[current Margin specification](https://developers.binance.com/en/docs/products/margin-trading/listen-token-data-stream).

`PortfolioMarginStreamSetup`, `PortfolioMarginProStreamSetup` and
`MarginRiskStreamSetup` expose dedicated connections for explicit registration.
Enabled futures or margin assets do not establish account eligibility for these
separate streams. Their offline lifecycle tests exercise the manager and the
correct PAPI, USD-M or Margin listen-key endpoint.

Unified USD-M conditional submissions now use the current UM algo endpoint and
return `algoId`. USD-M conditional cancellation selects the appropriate PAPI or
FAPI algo endpoint and preserves exact exchange IDs and client-only identifiers.
Offline regression tests reproduce the retired-route failure and check every
supported conditional order alias.

Endpoint-specific guards prevent shared request types from sending Spot-only
OCO fields to portfolio margin, UM-only fields to COIN-M, or conditional fields
to the ordinary USD-M order endpoint. The obsolete gift-card discount is rejected
before submission; zero is accepted. Unsupported history filters and test-only
SOR parameters have direct sentinel coverage. Margin stream closure uses only
the API-key header and accepts its documented empty success body while retaining
HTTP, API and malformed-response errors.
Portfolio-margin limit orders accept `priceMatch` without `price`. Simple Earn
subscriptions transmit explicit `autoSubscribe=false` instead of allowing the
server's default to override the caller's choice.

RPI depth updates are emitted as `FuturesRPIDepth`. They describe a different
liquidity universe and do not update the ordinary depth cache.

Existing saved pair formats and subscription choices are preserved. Subscription
expansion recognises generic and exchange channel names, explicit assets and
wildcards, and retains disabled entries. No configuration migration is required:
the changes interpret existing settings and add explicitly selected connections;
they do not rewrite saved choices. Newly initialised COIN-M pair formatting uses
the documented underscore separator.

## Legacy limitations

The legacy inventory distinguishes endpoint removal, deprecation, product
retirement and absence of a current published contract. Confirmed retirements
have GoDoc notices; compatibility helpers remain available. For example, the
[wallet changelog](https://developers.binance.com/en/docs/products/wallet/change-log)
documents BUSD conversion's failure response, which is now preserved and tested.

Current parameter and response contracts could not be revalidated for ten
legacy helpers: BUSD conversion history, the old fixed-loan collateral rate,
the separate flexible-loan collateral repayment, BETH reward history, two
Options transaction-download calls, the old Options account route, futures
historical orderbook downloads, and two wallet/futures transfer calls.
Seventeen Auto-Invest helpers also lack current published contracts following
the [move to Convert Recurring](https://www.binance.com/en/support/announcement/detail/af6e55cae01d46f98a6520dfd9cd2252).
These routes are not certified as current or live-verified.

## Go versions and reproduction

The merged repository requires Go 1.27 and was checked with Go 1.27.1.
Relevant additions from [Go 1.25](https://go.dev/doc/go1.25),
[Go 1.26](https://go.dev/doc/go1.26) and [Go 1.27](https://go.dev/doc/go1.27)
are used where they simplify the implementation: `sync.WaitGroup.Go`,
`new(value)`, `reflect.Type.Fields`, `httptest.NewTestServer` and `synctest.Sleep`.
Optional struct values use `omitzero`; tests run with the standard and Sonic
JSON backends.

```sh
go test ./... -race -count=1
go test -tags sonic_on ./exchanges/binance ./types ./encoding/json -count=1
go vet ./...
golangci-lint run ./...
make gofumpt
make misc_checks
make markdownlint
```

Public production checks can be repeated without credentials:

```sh
BINANCE_LIVE_PUBLIC=1 go test ./exchanges/binance -run '^TestPublicEndpointsLive$' -count=1 -v
BINANCE_LIVE_PUBLIC=1 go test ./exchanges/binance -run '^TestPublicWebsocketAPILive$' -count=1 -v
```
