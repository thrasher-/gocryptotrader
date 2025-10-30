package hyperliquid

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	json "github.com/thrasher-corp/gocryptotrader/encoding/json"
	exchange "github.com/thrasher-corp/gocryptotrader/exchanges"
)

const (
	testPrivateKey = "0x4f3edf983ac636a65a842ce7c78d9aa706d3b113b37ad5dee0c90c0f0da58c16"
	testAddress    = "0x90f8bf6a479f320ead074411a4b0e7944ea8c9c1"
	testVault      = "0x1111111111111111111111111111111111111111"
	metaPayload    = `{"universe":[{"name":"BTC","szDecimals":5,"maxLeverage":40,"marginTableId":1,"onlyIsolated":false,"isDelisted":false}]}`
)

func requireMap(t *testing.T, value any) map[string]any {
	t.Helper()
	m, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any, got %T", value)
	}
	return m
}

func requireSlice(t *testing.T, value any) []any {
	t.Helper()
	s, ok := value.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", value)
	}
	return s
}

func requireString(t *testing.T, value any) string {
	t.Helper()
	s, ok := value.(string)
	if !ok {
		t.Fatalf("expected string, got %T", value)
	}
	return s
}

func createSignedExchange(t *testing.T, onExchange func(map[string]any)) *Exchange {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer mustCloseBody(t, r.Body)
		var payload map[string]any
		mustDecodeJSON(t, r.Body, &payload)

		switch r.URL.Path {
		case infoPath:
			if payload["type"] != "meta" {
				t.Fatalf("expected meta request type, got %v", payload["type"])
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(metaPayload))
		case "/exchange":
			onExchange(payload)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	e := new(Exchange)
	e.SetDefaults()
	require.NoError(t, e.API.Endpoints.SetRunningURL(exchange.RestSpot.String(), server.URL))
	require.NoError(t, e.Requester.SetHTTPClient(server.Client()))
	e.now = func() time.Time { return time.UnixMilli(1700000000000) }
	e.SetCredentials(testAddress, testPrivateKey, "", testVault, "", "")
	return e
}

func capturePayload(t *testing.T, fn func(*Exchange) (*ExchangeResponse, error)) map[string]any {
	t.Helper()
	var captured map[string]any
	e := createSignedExchange(t, func(payload map[string]any) { captured = payload })
	resp, err := fn(e)
	require.NoError(t, err)
	require.NotNil(t, captured)
	require.NotNil(t, resp)
	require.Equal(t, "ok", resp.Status)
	return captured
}

func capturePayloadWithAgentKey(t *testing.T, fn func(*Exchange) (*ExchangeResponse, string, error)) (captured map[string]any, agentKey string) {
	t.Helper()
	e := createSignedExchange(t, func(payload map[string]any) { captured = payload })
	resp, agentKey, err := fn(e)
	require.NoError(t, err)
	require.NotNil(t, captured)
	require.NotNil(t, resp)
	require.Equal(t, "ok", resp.Status)
	return captured, agentKey
}

func ptrToInt(v int) *int {
	return &v
}

func ptrToString(v string) *string {
	return &v
}

func ptrToBool(v bool) *bool {
	return &v
}

func TestEnsureExchangeResponseOK(t *testing.T) {
	t.Parallel()

	require.NoError(t, ensureExchangeResponseOK(&ExchangeResponse{Status: "ok"}))
	require.ErrorIs(t, ensureExchangeResponseOK(nil), errResponseMissing)
	require.ErrorIs(t, ensureExchangeResponseOK(&ExchangeResponse{Status: "failure"}), errActionStatusNotOK)
}

func TestPlaceOrders(t *testing.T) {
	var captured map[string]any
	e := createSignedExchange(t, func(payload map[string]any) { captured = payload })

	orderReq := OrderRequest{
		Coin:       "BTC",
		IsBuy:      true,
		Size:       0.5,
		LimitPrice: 123.45,
		OrderType:  OrderType{Limit: &LimitOrderType{TimeInForce: "Gtc"}},
	}

	resp, err := e.PlaceOrders(context.Background(), []OrderRequest{orderReq}, nil)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, "ok", resp.Status)

	action := requireMap(t, captured["action"])
	require.Equal(t, "order", action["type"])
	orders := requireSlice(t, action["orders"])
	require.Len(t, orders, 1)
	orderWire := requireMap(t, orders[0])
	require.Equal(t, float64(0), orderWire["a"])
	require.Equal(t, "123.45", orderWire["p"])
	require.Equal(t, "0.5", orderWire["s"])
	require.Equal(t, strings.ToLower(testVault), requireString(t, captured["vaultAddress"]))
	sig := requireMap(t, captured["signature"])
	require.Contains(t, sig, "r")
	require.Contains(t, sig, "s")
	require.Contains(t, sig, "v")
}

func TestAmendOrders(t *testing.T) {
	var captured map[string]any
	e := createSignedExchange(t, func(payload map[string]any) { captured = payload })

	orderID := int64(99)
	req := ModifyRequest{
		Identifier: ModifyIdentifier{OrderID: &orderID},
		Order: OrderRequest{
			Coin:       "BTC",
			IsBuy:      false,
			Size:       1.25,
			LimitPrice: 321.0,
			OrderType:  OrderType{Limit: &LimitOrderType{TimeInForce: "Gtc"}},
			ReduceOnly: true,
		},
	}

	_, err := e.AmendOrders(context.Background(), []ModifyRequest{req})
	require.NoError(t, err)

	action := requireMap(t, captured["action"])
	require.Equal(t, "batchModify", action["type"])
	modifies := requireSlice(t, action["modifies"])
	require.Len(t, modifies, 1)
	entry := requireMap(t, modifies[0])
	require.Equal(t, float64(orderID), entry["oid"])
}

func TestCancelOrdersByID(t *testing.T) {
	var captured map[string]any
	e := createSignedExchange(t, func(payload map[string]any) { captured = payload })

	oid := int64(42)
	req := CancelRequest{Coin: "BTC", OrderID: &oid}
	_, err := e.CancelOrdersByID(context.Background(), []CancelRequest{req})
	require.NoError(t, err)

	action := requireMap(t, captured["action"])
	require.Equal(t, "cancel", action["type"])
	cancels := requireSlice(t, action["cancels"])
	require.Len(t, cancels, 1)
	entry := requireMap(t, cancels[0])
	require.Equal(t, float64(0), entry["a"])
	require.Equal(t, float64(oid), entry["o"])
}

func TestScheduleCancel(t *testing.T) {
	var captured map[string]any
	e := createSignedExchange(t, func(payload map[string]any) { captured = payload })

	ts := uint64(1700000005000)
	_, err := e.ScheduleCancel(context.Background(), &ts)
	require.NoError(t, err)

	action := requireMap(t, captured["action"])
	require.Equal(t, "scheduleCancel", action["type"])
	require.Equal(t, float64(ts), action["time"])
}

func TestUpdateLeverage(t *testing.T) {
	var captured map[string]any
	e := createSignedExchange(t, func(payload map[string]any) { captured = payload })

	_, err := e.UpdateLeverage(context.Background(), "BTC", 20, true)
	require.NoError(t, err)

	action := requireMap(t, captured["action"])
	require.Equal(t, "updateLeverage", action["type"])
	require.Equal(t, float64(0), action["asset"])
	require.Equal(t, float64(20), action["leverage"])
	require.Equal(t, true, action["isCross"])
}

func TestUpdateIsolatedMargin(t *testing.T) {
	var captured map[string]any
	e := createSignedExchange(t, func(payload map[string]any) { captured = payload })

	_, err := e.UpdateIsolatedMargin(context.Background(), "BTC", 1.2345, true)
	require.NoError(t, err)

	action := requireMap(t, captured["action"])
	require.Equal(t, "updateIsolatedMargin", action["type"])
	require.Equal(t, float64(0), action["asset"])
	require.Equal(t, true, action["isBuy"])
	require.Equal(t, float64(1234500), action["ntli"])
}

func TestSetReferrer(t *testing.T) {
	captured := capturePayload(t, func(e *Exchange) (*ExchangeResponse, error) {
		return e.SetReferrer(context.Background(), &SetReferrerRequest{Code: "REFER"})
	})
	action := requireMap(t, captured["action"])
	require.Equal(t, "setReferrer", action["type"])
	require.Equal(t, "REFER", action["code"])
	require.Nil(t, captured["vaultAddress"])
}

func TestCreateSubAccount(t *testing.T) {
	captured := capturePayload(t, func(e *Exchange) (*ExchangeResponse, error) {
		return e.CreateSubAccount(context.Background(), &CreateSubAccountRequest{Name: "Funding"})
	})
	action := requireMap(t, captured["action"])
	require.Equal(t, "createSubAccount", action["type"])
	require.Equal(t, "Funding", action["name"])
	require.Nil(t, captured["vaultAddress"])
}

func TestUSDClassTransfer(t *testing.T) {
	captured := capturePayload(t, func(e *Exchange) (*ExchangeResponse, error) {
		return e.USDClassTransfer(context.Background(), &USDClassTransferRequest{Amount: 12.34, ToPerp: true})
	})
	action := requireMap(t, captured["action"])
	require.Equal(t, "usdClassTransfer", action["type"])
	require.Equal(t, "12.34 subaccount:"+strings.ToLower(testVault), action["amount"])
	require.Equal(t, true, action["toPerp"])
	require.Equal(t, "0x18bcfe56800", action["nonce"])
	require.Nil(t, captured["vaultAddress"])
}

func TestSendAsset(t *testing.T) {
	captured := capturePayload(t, func(e *Exchange) (*ExchangeResponse, error) {
		req := &SendAssetRequest{
			Destination:    "0xABCDEF",
			SourceDEX:      "perp",
			DestinationDEX: "spot",
			Token:          "USDC",
			Amount:         1.5,
		}
		return e.SendAsset(context.Background(), req)
	})
	action := requireMap(t, captured["action"])
	require.Equal(t, "sendAsset", action["type"])
	require.Equal(t, strings.ToLower("0xABCDEF"), action["destination"])
	require.Equal(t, "perp", action["sourceDex"])
	require.Equal(t, "spot", action["destinationDex"])
	require.Equal(t, "USDC", action["token"])
	require.Equal(t, "1.5", action["amount"])
	require.Equal(t, strings.ToLower(testVault), action["fromSubAccount"])
	require.Equal(t, "0x18bcfe56800", action["nonce"])
	require.Nil(t, captured["vaultAddress"])
}

func TestSubAccountTransfers(t *testing.T) {
	transferCaptured := capturePayload(t, func(e *Exchange) (*ExchangeResponse, error) {
		return e.SubAccountTransfer(context.Background(), &SubAccountTransferRequest{
			SubAccountUser: "0xAABBCC",
			IsDeposit:      true,
			USD:            1000,
		})
	})
	action := requireMap(t, transferCaptured["action"])
	require.Equal(t, "subAccountTransfer", action["type"])
	require.Equal(t, strings.ToLower("0xAABBCC"), action["subAccountUser"])
	require.Equal(t, true, action["isDeposit"])
	require.Equal(t, float64(1000), action["usd"])

	spotCaptured := capturePayload(t, func(e *Exchange) (*ExchangeResponse, error) {
		return e.SubAccountSpotTransfer(context.Background(), &SubAccountSpotTransferRequest{
			SubAccountUser: "0xAABBCC",
			IsDeposit:      false,
			Token:          "USDC",
			Amount:         2.5,
		})
	})
	spotAction := requireMap(t, spotCaptured["action"])
	require.Equal(t, "subAccountSpotTransfer", spotAction["type"])
	require.Equal(t, strings.ToLower("0xAABBCC"), spotAction["subAccountUser"])
	require.Equal(t, false, spotAction["isDeposit"])
	require.Equal(t, "USDC", spotAction["token"])
	require.Equal(t, "2.5", spotAction["amount"])
}

func TestVaultUSDTransfer(t *testing.T) {
	captured := capturePayload(t, func(e *Exchange) (*ExchangeResponse, error) {
		return e.VaultUSDTransfer(context.Background(), &VaultUSDTransferRequest{
			VaultAddress: "0xBBBB",
			IsDeposit:    true,
			USD:          555,
		})
	})
	action := requireMap(t, captured["action"])
	require.Equal(t, "vaultTransfer", action["type"])
	require.Equal(t, strings.ToLower("0xBBBB"), action["vaultAddress"])
	require.Equal(t, true, action["isDeposit"])
	require.Equal(t, float64(555), action["usd"])
	require.Nil(t, captured["vaultAddress"])
}

func TestUSDTransfer(t *testing.T) {
	captured := capturePayload(t, func(e *Exchange) (*ExchangeResponse, error) {
		return e.USDTransfer(context.Background(), &USDTransferRequest{Destination: "0xABC", Amount: 5})
	})
	action := requireMap(t, captured["action"])
	require.Equal(t, "usdSend", action["type"])
	require.Equal(t, strings.ToLower("0xABC"), action["destination"])
	require.Equal(t, "5", action["amount"])
	require.Equal(t, "0x18bcfe56800", action["time"])
	require.Equal(t, strings.ToLower(testVault), requireString(t, captured["vaultAddress"]))
}

func TestSpotTransfer(t *testing.T) {
	captured := capturePayload(t, func(e *Exchange) (*ExchangeResponse, error) {
		return e.SpotTransfer(context.Background(), &SpotTransferRequest{Destination: "0xF00", Token: "USDC", Amount: 7.25})
	})
	action := requireMap(t, captured["action"])
	require.Equal(t, "spotSend", action["type"])
	require.Equal(t, strings.ToLower("0xF00"), action["destination"])
	require.Equal(t, "USDC", action["token"])
	require.Equal(t, "7.25", action["amount"])
	require.Equal(t, "0x18bcfe56800", action["time"])
	require.Equal(t, strings.ToLower(testVault), requireString(t, captured["vaultAddress"]))
}

func TestTokenDelegate(t *testing.T) {
	captured := capturePayload(t, func(e *Exchange) (*ExchangeResponse, error) {
		return e.TokenDelegate(context.Background(), &TokenDelegateRequest{Validator: testAddress, Wei: 10, IsUndelegate: true})
	})
	action := requireMap(t, captured["action"])
	require.Equal(t, "tokenDelegate", action["type"])
	require.Equal(t, strings.ToLower(testAddress), action["validator"])
	require.Equal(t, "0xa", action["wei"])
	require.Equal(t, true, action["isUndelegate"])
	require.Equal(t, "0x18bcfe56800", action["nonce"])
	require.Equal(t, strings.ToLower(testVault), requireString(t, captured["vaultAddress"]))
}

func TestWithdrawFromBridge(t *testing.T) {
	captured := capturePayload(t, func(e *Exchange) (*ExchangeResponse, error) {
		return e.WithdrawFromBridge(context.Background(), &WithdrawFromBridgeRequest{Destination: "0xFACE", Amount: 9.5})
	})
	action := requireMap(t, captured["action"])
	require.Equal(t, "withdraw3", action["type"])
	require.Equal(t, strings.ToLower("0xFACE"), action["destination"])
	require.Equal(t, "9.5", action["amount"])
	require.Equal(t, "0x18bcfe56800", action["time"])
	require.Equal(t, strings.ToLower(testVault), requireString(t, captured["vaultAddress"]))
}

func TestApproveAgent(t *testing.T) {
	captured, agentKey := capturePayloadWithAgentKey(t, func(e *Exchange) (*ExchangeResponse, string, error) {
		return e.ApproveAgent(context.Background(), &ApproveAgentRequest{AgentName: "helper"})
	})
	require.Len(t, agentKey, 66)
	require.True(t, strings.HasPrefix(agentKey, "0x"))
	action := requireMap(t, captured["action"])
	require.Equal(t, "approveAgent", action["type"])
	require.Equal(t, "helper", action["agentName"])
	agentAddr := requireString(t, action["agentAddress"])
	require.True(t, strings.HasPrefix(agentAddr, "0x"))
	require.Equal(t, "0x18bcfe56800", action["nonce"])
	require.Equal(t, strings.ToLower(testVault), requireString(t, captured["vaultAddress"]))
}

func TestApproveBuilderFee(t *testing.T) {
	captured := capturePayload(t, func(e *Exchange) (*ExchangeResponse, error) {
		return e.ApproveBuilderFee(context.Background(), &ApproveBuilderFeeRequest{Builder: testAddress, MaxFeeRate: "0.001"})
	})
	action := requireMap(t, captured["action"])
	require.Equal(t, "approveBuilderFee", action["type"])
	require.Equal(t, strings.ToLower(testAddress), action["builder"])
	require.Equal(t, "0.001", action["maxFeeRate"])
	require.Equal(t, "0x18bcfe56800", action["nonce"])
	require.Equal(t, strings.ToLower(testVault), requireString(t, captured["vaultAddress"]))
}

func TestConvertToMultiSigUser(t *testing.T) {
	captured := capturePayload(t, func(e *Exchange) (*ExchangeResponse, error) {
		req := &ConvertToMultiSigUserRequest{
			AuthorizedUsers: []string{"0x2", "0x1"},
			Threshold:       2,
		}
		return e.ConvertToMultiSigUser(context.Background(), req)
	})
	action := requireMap(t, captured["action"])
	require.Equal(t, "convertToMultiSigUser", action["type"])
	signersJSON := requireString(t, action["signers"])
	var payload struct {
		AuthorizedUsers []string `json:"authorizedUsers"`
		Threshold       int      `json:"threshold"`
	}
	require.NoError(t, json.Unmarshal([]byte(signersJSON), &payload))
	require.Equal(t, []string{"0x1", "0x2"}, payload.AuthorizedUsers)
	require.Equal(t, 2, payload.Threshold)
	require.Equal(t, "0x18bcfe56800", action["nonce"])
	require.Equal(t, strings.ToLower(testVault), requireString(t, captured["vaultAddress"]))
}

func TestSpotDeployRegisterToken(t *testing.T) {
	captured := capturePayload(t, func(e *Exchange) (*ExchangeResponse, error) {
		req := &SpotDeployRegisterTokenRequest{
			TokenName:    "TKN",
			SizeDecimals: 8,
			WeiDecimals:  18,
			MaxGas:       12345,
			FullName:     "Token",
		}
		return e.SpotDeployRegisterToken(context.Background(), req)
	})
	action := requireMap(t, captured["action"])
	require.Equal(t, "spotDeploy", action["type"])
	register := requireMap(t, action["registerToken2"])
	spec := requireMap(t, register["spec"])
	require.Equal(t, "TKN", spec["name"])
	require.Equal(t, float64(8), spec["szDecimals"])
	require.Equal(t, float64(18), spec["weiDecimals"])
	require.Equal(t, float64(12345), register["maxGas"])
	require.Equal(t, "Token", register["fullName"])
}

func TestSpotDeployUserGenesis(t *testing.T) {
	captured := capturePayload(t, func(e *Exchange) (*ExchangeResponse, error) {
		req := &SpotDeployUserGenesisRequest{
			Token:               1,
			UserAndWei:          []SpotDeployUserGenesisEntry{{User: "0xAAA", Wei: "10"}},
			ExistingTokenAndWei: []SpotDeployExistingTokenWeiEntry{{Token: 2, Wei: "5"}},
		}
		return e.SpotDeployUserGenesis(context.Background(), req)
	})
	action := requireMap(t, captured["action"])
	require.Equal(t, "spotDeploy", action["type"])
	genesis := requireMap(t, action["userGenesis"])
	require.Equal(t, float64(1), genesis["token"])
	users := requireSlice(t, genesis["userAndWei"])
	require.Equal(t, []any{strings.ToLower("0xAAA"), "10"}, requireSlice(t, users[0]))
	existing := requireSlice(t, genesis["existingTokenAndWei"])
	require.Equal(t, []any{float64(2), "5"}, requireSlice(t, existing[0]))
}

func TestSpotDeployTokenActions(t *testing.T) {
	tests := []struct {
		name    string
		invoke  func(*Exchange) (*ExchangeResponse, error)
		variant string
		assert  func(*testing.T, map[string]any)
	}{
		{
			name: "EnableFreezePrivilege",
			invoke: func(e *Exchange) (*ExchangeResponse, error) {
				return e.SpotDeployEnableFreezePrivilege(context.Background(), 4)
			},
			variant: "enableFreezePrivilege",
			assert: func(t *testing.T, action map[string]any) {
				t.Helper()
				require.Equal(t, float64(4), requireMap(t, action["enableFreezePrivilege"])["token"])
			},
		},
		{
			name: "FreezeUser",
			invoke: func(e *Exchange) (*ExchangeResponse, error) {
				return e.SpotDeployFreezeUser(context.Background(), &SpotDeployFreezeUserRequest{Token: 5, User: "0xCCC", Freeze: true})
			},
			variant: "freezeUser",
			assert: func(t *testing.T, action map[string]any) {
				t.Helper()
				freeze := requireMap(t, action["freezeUser"])
				require.Equal(t, float64(5), freeze["token"])
				require.Equal(t, strings.ToLower("0xCCC"), freeze["user"])
				require.Equal(t, true, freeze["freeze"])
			},
		},
		{
			name: "RevokeFreezePrivilege",
			invoke: func(e *Exchange) (*ExchangeResponse, error) {
				return e.SpotDeployRevokeFreezePrivilege(context.Background(), 6)
			},
			variant: "revokeFreezePrivilege",
			assert: func(t *testing.T, action map[string]any) {
				t.Helper()
				require.Equal(t, float64(6), requireMap(t, action["revokeFreezePrivilege"])["token"])
			},
		},
		{
			name: "EnableQuoteToken",
			invoke: func(e *Exchange) (*ExchangeResponse, error) {
				return e.SpotDeployEnableQuoteToken(context.Background(), 7)
			},
			variant: "enableQuoteToken",
			assert: func(t *testing.T, action map[string]any) {
				t.Helper()
				require.Equal(t, float64(7), requireMap(t, action["enableQuoteToken"])["token"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			captured := capturePayload(t, tt.invoke)
			action := requireMap(t, captured["action"])
			require.Equal(t, "spotDeploy", action["type"])
			tt.assert(t, action)
		})
	}
}

func TestSpotDeployGenesis(t *testing.T) {
	captured := capturePayload(t, func(e *Exchange) (*ExchangeResponse, error) {
		return e.SpotDeployGenesis(context.Background(), &SpotDeployGenesisRequest{Token: 9, MaxSupply: "999", NoHyperliquidity: true})
	})
	action := requireMap(t, captured["action"])
	require.Equal(t, "spotDeploy", action["type"])
	genesis := requireMap(t, action["genesis"])
	require.Equal(t, float64(9), genesis["token"])
	require.Equal(t, "999", genesis["maxSupply"])
	require.Equal(t, true, genesis["noHyperliquidity"])
}

func TestSpotDeployRegisterSpot(t *testing.T) {
	captured := capturePayload(t, func(e *Exchange) (*ExchangeResponse, error) {
		return e.SpotDeployRegisterSpot(context.Background(), &SpotDeployRegisterSpotRequest{BaseToken: 1, QuoteToken: 2})
	})
	action := requireMap(t, captured["action"])
	require.Equal(t, "spotDeploy", action["type"])
	register := requireMap(t, action["registerSpot"])
	require.Equal(t, float64(1), register["baseToken"])
	require.Equal(t, float64(2), register["quoteToken"])
}

func TestSpotDeployRegisterHyperliquidity(t *testing.T) {
	nSeeded := 3
	captured := capturePayload(t, func(e *Exchange) (*ExchangeResponse, error) {
		req := &SpotDeployRegisterHyperliquidityRequest{Spot: 4, StartPrice: 1.1, OrderSize: 0.5, Orders: 10, SeededLevels: &nSeeded}
		return e.SpotDeployRegisterHyperliquidity(context.Background(), req)
	})
	action := requireMap(t, captured["action"])
	require.Equal(t, "spotDeploy", action["type"])
	register := requireMap(t, action["registerHyperliquidity"])
	require.Equal(t, float64(4), register["spot"])
	require.Equal(t, "1.1", register["startPx"])
	require.Equal(t, "0.5", register["orderSz"])
	require.Equal(t, float64(10), register["nOrders"])
	require.Equal(t, float64(nSeeded), register["nSeededLevels"])
}

func TestSpotDeploySetDeployerTradingFeeShare(t *testing.T) {
	captured := capturePayload(t, func(e *Exchange) (*ExchangeResponse, error) {
		return e.SpotDeploySetDeployerTradingFeeShare(context.Background(), &SpotDeploySetDeployerTradingFeeShareRequest{Token: 8, Share: "0.25"})
	})
	action := requireMap(t, captured["action"])
	require.Equal(t, "spotDeploy", action["type"])
	setShare := requireMap(t, action["setDeployerTradingFeeShare"])
	require.Equal(t, float64(8), setShare["token"])
	require.Equal(t, "0.25", setShare["share"])
}

func TestPerpDeployRegisterAsset(t *testing.T) {
	captured := capturePayload(t, func(e *Exchange) (*ExchangeResponse, error) {
		req := &PerpDeployRegisterAssetRequest{
			Dex:           "main",
			MaxGas:        ptrToInt(5000),
			Coin:          "BTC",
			SizeDecimals:  3,
			OraclePrice:   "50000",
			MarginTableID: 2,
			OnlyIsolated:  true,
			Schema: &PerpDeploySchema{
				FullName:        "Bitcoin",
				CollateralToken: "USDC",
				OracleUpdater:   ptrToString("0xfeed"),
			},
		}
		return e.PerpDeployRegisterAsset(context.Background(), req)
	})
	action := requireMap(t, captured["action"])
	require.Equal(t, "perpDeploy", action["type"])
	register := requireMap(t, action["registerAsset"])
	require.Equal(t, "main", register["dex"])
	require.Equal(t, float64(5000), register["maxGas"])
	asset := requireMap(t, register["assetRequest"])
	require.Equal(t, "BTC", asset["coin"])
	require.Equal(t, float64(3), asset["szDecimals"])
	require.Equal(t, "50000", asset["oraclePx"])
	require.Equal(t, float64(2), asset["marginTableId"])
	require.Equal(t, true, asset["onlyIsolated"])
	schema := requireMap(t, register["schema"])
	require.Equal(t, "Bitcoin", schema["fullName"])
	require.Equal(t, "USDC", schema["collateralToken"])
	require.Equal(t, strings.ToLower("0xfeed"), schema["oracleUpdater"])
}

func TestPerpDeploySetOracle(t *testing.T) {
	captured := capturePayload(t, func(e *Exchange) (*ExchangeResponse, error) {
		req := &PerpDeploySetOracleRequest{
			Dex:                "main",
			OraclePrices:       map[string]string{"BTC": "50000", "ETH": "3000"},
			MarkPrices:         []map[string]string{{"BTC": "50010", "ETH": "3010"}},
			ExternalPerpPrices: map[string]string{"BTC": "49990"},
		}
		return e.PerpDeploySetOracle(context.Background(), req)
	})
	action := requireMap(t, captured["action"])
	require.Equal(t, "perpDeploy", action["type"])
	setOracle := requireMap(t, action["setOracle"])
	require.Equal(t, "main", setOracle["dex"])
	oracle := requireSlice(t, setOracle["oraclePxs"])
	require.Equal(t, []any{"BTC", "50000"}, requireSlice(t, oracle[0]))
	marks := requireSlice(t, setOracle["markPxs"])
	markEntry := requireSlice(t, marks[0])
	require.Equal(t, []any{"BTC", "50010"}, requireSlice(t, markEntry[0]))
	external := requireSlice(t, setOracle["externalPerpPxs"])
	require.Equal(t, []any{"BTC", "49990"}, requireSlice(t, external[0]))
}

func TestCSignerActions(t *testing.T) {
	tests := []struct {
		name    string
		invoke  func(*Exchange) (*ExchangeResponse, error)
		variant string
	}{
		{
			name:    "Unjail",
			invoke:  func(e *Exchange) (*ExchangeResponse, error) { return e.CSignerUnjailSelf(context.Background()) },
			variant: "unjailSelf",
		},
		{
			name:    "Jail",
			invoke:  func(e *Exchange) (*ExchangeResponse, error) { return e.CSignerJailSelf(context.Background()) },
			variant: "jailSelf",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			captured := capturePayload(t, tt.invoke)
			action := requireMap(t, captured["action"])
			require.Equal(t, "CSignerAction", action["type"])
			require.Contains(t, action, tt.variant)
		})
	}
}

func TestCValidatorRegister(t *testing.T) {
	captured := capturePayload(t, func(e *Exchange) (*ExchangeResponse, error) {
		req := &CValidatorRegisterRequest{
			NodeIP:              "1.2.3.4",
			Name:                "Validator",
			Description:         "desc",
			DelegationsDisabled: true,
			CommissionBPS:       50,
			Signer:              "0xAAA",
			Unjailed:            true,
			InitialWei:          100,
		}
		return e.CValidatorRegister(context.Background(), req)
	})
	action := requireMap(t, captured["action"])
	require.Equal(t, "CValidatorAction", action["type"])
	register := requireMap(t, action["register"])
	profile := requireMap(t, register["profile"])
	require.Equal(t, map[string]any{"Ip": "1.2.3.4"}, requireMap(t, profile["node_ip"]))
	require.Equal(t, "Validator", profile["name"])
	require.Equal(t, "desc", profile["description"])
	require.Equal(t, true, profile["delegations_disabled"])
	require.Equal(t, float64(50), profile["commission_bps"])
	require.Equal(t, strings.ToLower("0xAAA"), profile["signer"])
	require.Equal(t, true, register["unjailed"])
	require.Equal(t, float64(100), register["initial_wei"])
}

func TestCValidatorChangeProfile(t *testing.T) {
	captured := capturePayload(t, func(e *Exchange) (*ExchangeResponse, error) {
		req := &CValidatorChangeProfileRequest{
			NodeIP:             ptrToString("5.6.7.8"),
			Name:               ptrToString("New"),
			Description:        ptrToString("updated"),
			Unjailed:           false,
			DisableDelegations: ptrToBool(true),
			CommissionBPS:      ptrToInt(60),
			Signer:             ptrToString("0xBBB"),
		}
		return e.CValidatorChangeProfile(context.Background(), req)
	})
	action := requireMap(t, captured["action"])
	require.Equal(t, "CValidatorAction", action["type"])
	change := requireMap(t, action["changeProfile"])
	nodeIP := requireMap(t, change["node_ip"])
	require.Equal(t, "5.6.7.8", nodeIP["Ip"])
	require.Equal(t, "New", change["name"])
	require.Equal(t, "updated", change["description"])
	require.Equal(t, false, change["unjailed"])
	require.Equal(t, true, change["disable_delegations"])
	require.Equal(t, float64(60), change["commission_bps"])
	require.Equal(t, strings.ToLower("0xBBB"), change["signer"])
}

func TestCValidatorUnregister(t *testing.T) {
	captured := capturePayload(t, func(e *Exchange) (*ExchangeResponse, error) {
		return e.CValidatorUnregister(context.Background())
	})
	action := requireMap(t, captured["action"])
	require.Equal(t, "CValidatorAction", action["type"])
	require.Contains(t, action, "unregister")
}

func TestMultiSig(t *testing.T) {
	captured := capturePayload(t, func(e *Exchange) (*ExchangeResponse, error) {
		req := &MultiSigRequest{
			MultiSigUser: "0xABC",
			Action:       map[string]any{"type": "noop"},
			Signatures:   []MultiSigSignature{{R: "0x1", S: "0x2", V: 27}},
			Nonce:        123,
			VaultAddress: ptrToString("0xDEF0"),
		}
		return e.MultiSig(context.Background(), req)
	})
	action := requireMap(t, captured["action"])
	require.Equal(t, "multiSig", action["type"])
	require.Equal(t, defaultSignatureChainID, action["signatureChainId"])
	payload := requireMap(t, action["payload"])
	require.Equal(t, strings.ToLower("0xABC"), payload["multiSigUser"])
	require.Equal(t, map[string]any{"type": "noop"}, payload["action"])
	require.Equal(t, float64(123), captured["nonce"])
	require.Equal(t, strings.ToLower("0xDEF0"), requireString(t, captured["vaultAddress"]))
}

func TestUseBigBlocks(t *testing.T) {
	captured := capturePayload(t, func(e *Exchange) (*ExchangeResponse, error) {
		return e.UseBigBlocks(context.Background(), &UseBigBlocksRequest{Enable: true})
	})
	action := requireMap(t, captured["action"])
	require.Equal(t, "evmUserModify", action["type"])
	require.Equal(t, true, action["usingBigBlocks"])
	require.Nil(t, captured["vaultAddress"])
}

func TestAgentEnableDexAbstraction(t *testing.T) {
	captured := capturePayload(t, func(e *Exchange) (*ExchangeResponse, error) {
		return e.AgentEnableDexAbstraction(context.Background())
	})
	action := requireMap(t, captured["action"])
	require.Equal(t, "agentEnableDexAbstraction", action["type"])
	require.Equal(t, strings.ToLower(testVault), requireString(t, captured["vaultAddress"]))
}

func TestUserDexAbstraction(t *testing.T) {
	captured := capturePayload(t, func(e *Exchange) (*ExchangeResponse, error) {
		req := &UserDexAbstractionRequest{User: testAddress, Enabled: true}
		return e.UserDexAbstraction(context.Background(), req)
	})
	action := requireMap(t, captured["action"])
	require.Equal(t, "userDexAbstraction", action["type"])
	require.Equal(t, strings.ToLower(testAddress), action["user"])
	require.Equal(t, true, action["enabled"])
	require.Equal(t, "0x18bcfe56800", action["nonce"])
	require.Equal(t, strings.ToLower(testVault), requireString(t, captured["vaultAddress"]))
}

func TestNoop(t *testing.T) {
	captured := capturePayload(t, func(e *Exchange) (*ExchangeResponse, error) {
		return e.Noop(context.Background(), &NoopRequest{Nonce: 321})
	})
	action := requireMap(t, captured["action"])
	require.Equal(t, "noop", action["type"])
	require.Equal(t, float64(321), captured["nonce"])
	require.Equal(t, strings.ToLower(testVault), requireString(t, captured["vaultAddress"]))
}
