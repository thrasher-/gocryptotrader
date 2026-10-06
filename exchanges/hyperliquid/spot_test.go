package hyperliquid

import (
	"math"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/common"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/types"
)

// testSpotMetadata is the spot metadata the mock responses return
func testSpotMetadata() SpotMetadata {
	return SpotMetadata{
		Universe: []SpotAssetMetadata{
			{Tokens: []uint64{1, 0}, Name: "PURR/USDC", IsCanonical: true},
			{Tokens: []uint64{150, 0}, Name: "@107", Index: 107},
			{Tokens: []uint64{360, 0}, Name: "@230", Index: 230},
		},
		Tokens: []SpotTokenMetadata{
			{Name: currency.USDC, SizeDecimals: 8, WeiDecimals: 8, TokenID: "0x6d1e7cde53ba9467b783cb7c530ce054", IsCanonical: true, EVMContract: &EVMContract{Address: "0x6b9e773128f453f5c2c60935ee2de2cbc5390a24", EVMExtraWeiDecimals: -2}},
			{Name: currency.NewCode("PURR"), WeiDecimals: 5, Index: 1, TokenID: "0xc1fb593aeffbeb02f85e0308e9956a90", IsCanonical: true, EVMContract: &EVMContract{Address: "0x9b498c3c8a0b8cd8ba1d9851d40d186f1872b44e", EVMExtraWeiDecimals: 13}},
			{Name: currency.HYPE, SizeDecimals: 2, WeiDecimals: 8, Index: 150, TokenID: "0x0d01dc56dcaaca66ad901c959b4011ec", FullName: "Hyperliquid"},
			{Name: currency.NewCode("USDH"), SizeDecimals: 2, WeiDecimals: 8, Index: 360, TokenID: "0x54e00a5988577cb0b0c9ab0cb6ef7f4b", EVMContract: &EVMContract{Address: "0x111111a1a0667d36bd57c0a9f569b98057111111", EVMExtraWeiDecimals: -2}, FullName: "USDH", DeployerTradingFeeShare: 1},
		},
	}
}

func TestGetSpotMetadata(t *testing.T) {
	t.Parallel()
	result, err := e.GetSpotMetadata(t.Context())
	require.NoError(t, err, "GetSpotMetadata must not error")
	if mockTests {
		exp := testSpotMetadata()
		assert.Equal(t, &exp, result, "GetSpotMetadata should decode every field")
	} else {
		assert.True(t, result.Tokens[0].Name.Equal(currency.USDC), "GetSpotMetadata should list USDC as token 0")
	}

	_, err = newResponseServerExchange(t, `null`).GetSpotMetadata(t.Context())
	assert.ErrorIs(t, err, common.ErrNoResponse, "GetSpotMetadata should reject a null response")
}

func TestGetSpotMetadataAndAssetContexts(t *testing.T) {
	t.Parallel()
	result, err := e.GetSpotMetadataAndAssetContexts(t.Context())
	require.NoError(t, err, "GetSpotMetadataAndAssetContexts must not error")
	if mockTests {
		exp := &SpotMetadataAndAssetContextsResponse{
			Metadata: testSpotMetadata(),
			AssetContexts: []SpotAssetContext{
				{PreviousDayPrice: 0.16577, DayNotionalVolume: 2361343.4740200015, MarkPrice: 0.15049, MidPrice: 0.150495, CirculatingSupply: 594786002.8013399839, Coin: "PURR/USDC", TotalSupply: 594786014.1078599691, DayBaseVolume: 14727160},
				{PreviousDayPrice: 93.102, DayNotionalVolume: 63321924.1126599833, MarkPrice: 93.146, MidPrice: 93.1465, CirculatingSupply: 298462929.368832469, Coin: "@107", TotalSupply: 998883571.7642624378, DayBaseVolume: 677650.0300000001},
				{PreviousDayPrice: 1.0001, DayNotionalVolume: 1501, MarkPrice: 1, CirculatingSupply: 50000000, Coin: "@230", TotalSupply: 50000000, DayBaseVolume: 1501},
				{PreviousDayPrice: 0.48, DayNotionalVolume: 1029.6, MarkPrice: 0.52, MidPrice: 0.515, CirculatingSupply: 1000, Coin: "#10", TotalSupply: 1000, DayBaseVolume: 2024.5},
			},
		}
		assert.Equal(t, exp, result, "GetSpotMetadataAndAssetContexts should decode the metadata and every context, including coins outside the universe")
	} else {
		assert.NotEmpty(t, result.AssetContexts, "GetSpotMetadataAndAssetContexts should return contexts")
	}

	_, err = newResponseServerExchange(t, `null`).GetSpotMetadataAndAssetContexts(t.Context())
	assert.ErrorIs(t, err, common.ErrNoResponse, "GetSpotMetadataAndAssetContexts should reject a null response")
}

func TestGetSpotClearinghouseState(t *testing.T) {
	t.Parallel()
	_, err := e.GetSpotClearinghouseState(t.Context(), "invalid")
	require.ErrorIs(t, err, errInvalidAddress, "GetSpotClearinghouseState must reject an invalid address")

	result, err := e.GetSpotClearinghouseState(t.Context(), testTraderAddress)
	require.NoError(t, err, "GetSpotClearinghouseState must not error")
	if mockTests {
		exp := &SpotClearinghouseStateResponse{Balances: []SpotBalance{
			{Coin: currency.USDC, Total: 14.625485},
			{Coin: currency.HYPE, Token: 150, Total: 816.67225318, Hold: 2.5, EntryNotional: 19838.88581631},
		}}
		assert.Equal(t, exp, result, "GetSpotClearinghouseState should decode every balance field")
	}

	result, err = e.GetSpotClearinghouseState(t.Context(), testVaultAddress)
	require.NoError(t, err, "GetSpotClearinghouseState must not error for the vault")
	if mockTests {
		exp := &SpotClearinghouseStateResponse{
			Balances: []SpotBalance{
				{Coin: currency.USDC, Total: 100508.11272667, Hold: 49999.89, EntryNotional: 1},
				{Coin: currency.NewCode("USDH"), Token: 360},
			},
			TokenToAvailableAfterMaintenance: []TokenAmount{{Amount: 100508.11272667}, {Token: 360}},
		}
		assert.Equal(t, exp, result, "GetSpotClearinghouseState should decode a unified account's available amounts after maintenance")
	}

	_, err = newResponseServerExchange(t, `null`).GetSpotClearinghouseState(t.Context(), testTraderAddress)
	assert.ErrorIs(t, err, common.ErrNoResponse, "GetSpotClearinghouseState should reject a null response")
}

func TestGetSpotDeployState(t *testing.T) {
	t.Parallel()
	_, err := e.GetSpotDeployState(t.Context(), "invalid")
	require.ErrorIs(t, err, errInvalidAddress, "GetSpotDeployState must reject an invalid address")

	result, err := e.GetSpotDeployState(t.Context(), testTraderAddress)
	require.NoError(t, err, "GetSpotDeployState must not error")
	if mockTests {
		exp := &SpotDeployStateResponse{
			States: []SpotDeployState{{
				Token:                        354,
				Specification:                SpotTokenSpecification{Name: currency.NewCode("RED"), SizeDecimals: 2, WeiDecimals: 8},
				FullName:                     "RedStone",
				DeployerTradingFeeShare:      1,
				Spots:                        []uint64{717},
				MaxSupply:                    184467440737.0955200195,
				HyperliquidityGenesisBalance: 120000,
				TotalGenesisBalanceWei:       math.MaxUint64,
				UserGenesisBalances:          []UserBalance{{User: "0x2000000000000000000000000000000000000162", Balance: 184467440737.0955200195}},
				ExistingTokenGenesisBalances: []ExistingTokenBalance{{Token: 1, Balance: 300000}},
				BlacklistUsers:               []string{"0x0000000000000000000000000000000000000019"},
			}},
			GasAuction: GasAuction{StartTime: types.Time(time.Unix(1733929200, 0)), DurationSeconds: 111600, StartGas: 181305.90046, CurrentGas: 181300, EndGas: 181291.247358},
		}
		assert.Equal(t, exp, result, "GetSpotDeployState should decode every field, including a genesis balance of the maximum uint64 wei")
	} else {
		assert.NotZero(t, result.GasAuction.DurationSeconds, "GetSpotDeployState should return the spot deploy gas auction")
	}

	_, err = newResponseServerExchange(t, `null`).GetSpotDeployState(t.Context(), testTraderAddress)
	assert.ErrorIs(t, err, common.ErrNoResponse, "GetSpotDeployState should reject a null response")
}

func TestGetSpotPairDeployAuctionStatus(t *testing.T) {
	t.Parallel()
	result, err := e.GetSpotPairDeployAuctionStatus(t.Context())
	require.NoError(t, err, "GetSpotPairDeployAuctionStatus must not error")
	if mockTests {
		exp := &GasAuction{StartTime: types.Time(time.Unix(1791180000, 0)), DurationSeconds: 111600, StartGas: 500, CurrentGas: 500}
		assert.Equal(t, exp, result, "GetSpotPairDeployAuctionStatus should decode a running auction")
	} else {
		assert.NotZero(t, result.DurationSeconds, "GetSpotPairDeployAuctionStatus should return the auction's duration")
	}

	_, err = newResponseServerExchange(t, `null`).GetSpotPairDeployAuctionStatus(t.Context())
	assert.ErrorIs(t, err, common.ErrNoResponse, "GetSpotPairDeployAuctionStatus should reject a null response")
}

func TestGetTokenDetails(t *testing.T) {
	t.Parallel()
	_, err := e.GetTokenDetails(t.Context(), "HFUN")
	require.ErrorIs(t, err, errInvalidTokenID, "GetTokenDetails must reject a token name")

	result, err := e.GetTokenDetails(t.Context(), "0xBAF265EF389DA684513D98D68EDF4EAE")
	require.NoError(t, err, "GetTokenDetails must not error for an upper case token ID")
	if mockTests {
		exp := &TokenDetailsResponse{
			Name:              currency.NewCode("HFUN"),
			MaxSupply:         1000000,
			TotalSupply:       995724.63958273,
			CirculatingSupply: 995724.58875657,
			SizeDecimals:      2,
			WeiDecimals:       8,
			MidPrice:          12.583,
			MarkPrice:         12.573,
			PreviousDayPrice:  12.516,
			Genesis: &TokenGenesis{
				UserBalances:          []UserBalance{{User: "0x0000000000000000000000000000000000000015", Balance: 300000}, {User: "0xffffffffffffffffffffffffffffffffffffffff", Balance: 400000}},
				ExistingTokenBalances: []ExistingTokenBalance{{Token: 1, Balance: 300000}},
				BlacklistUsers:        []string{"0x000000000000000000000000000000000000001a"},
			},
			Deployer:                   "0x0000000000000000000000000000000000000015",
			DeployGas:                  500,
			DeployTime:                 ZonelessTime(time.Date(2024, 5, 16, 11, 27, 33, 468000000, time.UTC)),
			SeededUSDC:                 1500,
			NonCirculatingUserBalances: []UserBalance{{User: "0x0000000000000000000000000000000000000000", Balance: 0.01074413}, {User: "0x000000000000000000000000000000000000dead", Balance: 0.04008203}},
			FutureEmissions:            410984598.9547457695,
		}
		assert.Equal(t, exp, result, "GetTokenDetails should decode every field")
	} else {
		assert.True(t, result.Name.Equal(currency.NewCode("HFUN")), "GetTokenDetails should return the requested token")
	}

	result, err = e.GetTokenDetails(t.Context(), "0x6d1e7cde53ba9467b783cb7c530ce054")
	require.NoError(t, err, "GetTokenDetails must not error for USDC")
	if mockTests {
		exp := &TokenDetailsResponse{
			Name:                       currency.USDC,
			MaxSupply:                  1000000000000,
			TotalSupply:                396751657.3657590151,
			CirculatingSupply:          396751657.3657590151,
			SizeDecimals:               8,
			MidPrice:                   1,
			MarkPrice:                  1,
			PreviousDayPrice:           1,
			NonCirculatingUserBalances: []UserBalance{},
		}
		assert.Equal(t, exp, result, "GetTokenDetails should decode a protocol token without genesis or deployment details")
	} else {
		assert.Nil(t, result.Genesis, "GetTokenDetails should return USDC without a genesis distribution")
	}

	_, err = newResponseServerExchange(t, `null`).GetTokenDetails(t.Context(), "0x6d1e7cde53ba9467b783cb7c530ce054")
	assert.ErrorIs(t, err, common.ErrNoResponse, "GetTokenDetails should reject a null response")
}

func TestValidateTokenID(t *testing.T) {
	t.Parallel()
	for _, tokenID := range []string{"0x6d1e7cde53ba9467b783cb7c530ce054", "0X6D1E7CDE53BA9467B783CB7C530CE054"} {
		assert.NoErrorf(t, validateTokenID(tokenID), "validateTokenID should accept %s", tokenID)
	}
	for _, tokenID := range []string{"", "USDC", "0x6d1e7cde53ba9467b783cb7c530ce05", "0x6d1e7cde53ba9467b783cb7c530ce0544", "1x6d1e7cde53ba9467b783cb7c530ce054", "0x6d1e7cde53ba9467b783cb7c530ce05g"} {
		assert.ErrorIsf(t, validateTokenID(tokenID), errInvalidTokenID, "validateTokenID should reject %q", tokenID)
	}
}

func TestGetOutcomeMetadata(t *testing.T) {
	t.Parallel()
	result, err := e.GetOutcomeMetadata(t.Context())
	require.NoError(t, err, "GetOutcomeMetadata must not error")
	if mockTests {
		sides := []OutcomeSideSpecification{{Name: "Yes"}, {Name: "No"}}
		exp := &OutcomeMetadataResponse{
			Outcomes: []OutcomeSpecification{
				{Outcome: 1472, Name: "template fallback", Description: "other", SideSpecifications: sides, QuoteToken: currency.USDC, Venue: "out", DeployerFeeScale: 5},
				{Outcome: 9015, Name: "Recurring", Description: "class:priceBinary|underlying:BTC|expiry:20261007-0600|targetPrice:85501|period:1d", SideSpecifications: sides, QuoteToken: currency.USDC},
			},
			Questions: []OutcomeQuestion{{
				Question:             198,
				Name:                 "template:sportsTournamentWinner",
				Description:          "competition:English Premier League|officialSource:English Premier League|resolutionDeadline:20270605-1200|season:2026/2027|sport:football/soccer",
				FallbackOutcome:      1472,
				NamedOutcomes:        []uint64{1473, 1474, 1475, 1476, 1477, 1478},
				SettledNamedOutcomes: []uint64{1477},
			}},
			Deployers: []OutcomeDeployer{{
				Deployer:     "0x0000000000000000000000000000000000000017",
				Venue:        "out",
				SubDeployers: []SubDeployer{{Action: "settleOutcome", Addresses: []string{"0x0000000000000000000000000000000000000018"}}},
			}},
			FeeScale: 1,
		}
		assert.Equal(t, exp, result, "GetOutcomeMetadata should decode every field")
	} else {
		assert.Positive(t, result.FeeScale.Float64(), "GetOutcomeMetadata should return the fee scale")
	}

	_, err = newResponseServerExchange(t, `null`).GetOutcomeMetadata(t.Context())
	assert.ErrorIs(t, err, common.ErrNoResponse, "GetOutcomeMetadata should reject a null response")
}

func TestGetSettledOutcome(t *testing.T) {
	t.Parallel()
	result, err := e.GetSettledOutcome(t.Context(), 0)
	require.NoError(t, err, "GetSettledOutcome must not error for outcome 0")
	exp := &SettledOutcomeResponse{
		Specification: OutcomeSpecification{
			Name:               "Recurring",
			Description:        "class:priceBinary|underlying:BTC|expiry:20260503-0600|targetPrice:78213|period:1d",
			SideSpecifications: []OutcomeSideSpecification{{Name: "Yes"}, {Name: "No"}},
			QuoteToken:         currency.NewCode("USDH"),
		},
		Details: "price:78212.4",
	}
	assert.Equal(t, exp, result, "GetSettledOutcome should decode a settled protocol outcome")

	result, err = e.GetSettledOutcome(t.Context(), 9092)
	require.NoError(t, err, "GetSettledOutcome must not error for a deployer outcome")
	if mockTests {
		exp = &SettledOutcomeResponse{
			Specification: OutcomeSpecification{
				Outcome:            9092,
				Name:               "template:binaryPrice",
				Description:        "perp:BTC|priceDescription:the Hyperliquid BTC perp trade|seconds:90|threshold:85558|time:20261006-0900",
				SideSpecifications: []OutcomeSideSpecification{{Name: "template:Yes"}, {Name: "template:No"}},
				QuoteToken:         currency.USDC,
				Venue:              "skew",
				DeployerFeeScale:   1,
			},
			SettleFraction: 1,
			Details:        "template",
			Question: &SettledOutcomeQuestion{
				Question:    QuestionState{Question: 390, State: QuestionStateSettled},
				Name:        "Recurring",
				Description: "class:priceBucket|underlying:BTC|expiry:20261006-0600|priceThresholds:84034,87464|period:1d",
			},
		}
		assert.Equal(t, exp, result, "GetSettledOutcome should decode every field")
	} else {
		assert.Equal(t, uint64(9092), result.Specification.Outcome, "GetSettledOutcome should return the requested outcome")
	}

	var body map[string]any
	ex := newTestServerExchange(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&body), "Decoding the info request should not error") {
			return
		}
		_, err := w.Write([]byte(`null`))
		assert.NoError(t, err, "Writing the response should not error")
	}))
	_, err = ex.GetSettledOutcome(t.Context(), 0)
	assert.ErrorIs(t, err, common.ErrNoResponse, "GetSettledOutcome should report an unsettled outcome's null response")
	assert.Equal(t, map[string]any{"type": "settledOutcome", "outcome": float64(0)}, body, "GetSettledOutcome should send outcome 0, which Hyperliquid requires")
}

func TestGetOutcomeDeployerLimits(t *testing.T) {
	t.Parallel()
	_, err := e.GetOutcomeDeployerLimits(t.Context(), " ")
	require.ErrorIs(t, err, errVenueRequired, "GetOutcomeDeployerLimits must require a venue")

	result, err := e.GetOutcomeDeployerLimits(t.Context(), "out")
	require.NoError(t, err, "GetOutcomeDeployerLimits must not error")
	if mockTests {
		assert.Equal(t, &OutcomeDeployerLimitsResponse{NumberOfDailyOutcomesRemaining: 851, NumberOfActiveOutcomesRemaining: 27}, result, "GetOutcomeDeployerLimits should decode every field")
	}

	_, err = newResponseServerExchange(t, `null`).GetOutcomeDeployerLimits(t.Context(), "out")
	assert.ErrorIs(t, err, common.ErrNoResponse, "GetOutcomeDeployerLimits should reject a null response")
}

func TestSpotMetadataAndAssetContextsResponseUnmarshalJSON(t *testing.T) {
	t.Parallel()
	var response SpotMetadataAndAssetContextsResponse
	require.NoError(t, json.Unmarshal([]byte(`[{"tokens":[{"name":"USDC"}]},[{"coin":"#10","markPx":"0.52"}]]`), &response), "Unmarshal must not error")
	exp := SpotMetadataAndAssetContextsResponse{
		Metadata:      SpotMetadata{Tokens: []SpotTokenMetadata{{Name: currency.USDC}}},
		AssetContexts: []SpotAssetContext{{Coin: "#10", MarkPrice: 0.52}},
	}
	assert.Equal(t, exp, response, "SpotMetadataAndAssetContextsResponse should decode its metadata and contexts")
	assert.ErrorIs(t, json.Unmarshal([]byte(`[{}]`), &response), errInvalidTupleLength, "SpotMetadataAndAssetContextsResponse should reject a short tuple")
}

func TestTokenAmountUnmarshalJSON(t *testing.T) {
	t.Parallel()
	var amount TokenAmount
	require.NoError(t, json.Unmarshal([]byte(`[360,"100508.11272667"]`), &amount), "Unmarshal must not error")
	assert.Equal(t, TokenAmount{Token: 360, Amount: 100508.11272667}, amount, "TokenAmount should decode its token and amount")
	assert.ErrorIs(t, json.Unmarshal([]byte(`[360]`), &amount), errInvalidTupleLength, "TokenAmount should reject a short tuple")
}

func TestUserBalanceUnmarshalJSON(t *testing.T) {
	t.Parallel()
	var balance UserBalance
	require.NoError(t, json.Unmarshal([]byte(`["0x000000000000000000000000000000000000dead","0.04008203"]`), &balance), "Unmarshal must not error")
	assert.Equal(t, UserBalance{User: "0x000000000000000000000000000000000000dead", Balance: 0.04008203}, balance, "UserBalance should decode its user and balance")
	assert.ErrorIs(t, json.Unmarshal([]byte(`["0x000000000000000000000000000000000000dead"]`), &balance), errInvalidTupleLength, "UserBalance should reject a short tuple")
}

func TestExistingTokenBalanceUnmarshalJSON(t *testing.T) {
	t.Parallel()
	var balance ExistingTokenBalance
	require.NoError(t, json.Unmarshal([]byte(`[1,"300000.0"]`), &balance), "Unmarshal must not error")
	assert.Equal(t, ExistingTokenBalance{Token: 1, Balance: 300000}, balance, "ExistingTokenBalance should decode its token and balance")
	assert.ErrorIs(t, json.Unmarshal([]byte(`[1]`), &balance), errInvalidTupleLength, "ExistingTokenBalance should reject a short tuple")
}

func TestQuestionStateUnmarshalJSON(t *testing.T) {
	t.Parallel()
	for data, exp := range map[string]QuestionState{
		`{"active":823}`:  {Question: 823, State: QuestionStateActive},
		`{"settled":390}`: {Question: 390, State: QuestionStateSettled},
	} {
		var state QuestionState
		require.NoErrorf(t, json.Unmarshal([]byte(data), &state), "Unmarshal must not error for %s", data)
		assert.Equalf(t, exp, state, "QuestionState should decode %s", data)
	}
	for _, data := range []string{`{}`, `{"active":1,"settled":2}`, `[]`, `{"active":"1"}`} {
		var state QuestionState
		assert.ErrorIsf(t, json.Unmarshal([]byte(data), &state), errInvalidQuestionState, "QuestionState should reject %s", data)
	}
}
