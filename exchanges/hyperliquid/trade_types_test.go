package hyperliquid

import (
	"testing"

	"github.com/stretchr/testify/require"
	json "github.com/thrasher-corp/gocryptotrader/encoding/json"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
)

func TestExchangeResponseUnmarshal(t *testing.T) {
	payload := `{
		"status":"ok",
		"txHash":"0xtxhash",
		"response":{
			"type":"order",
			"data":{
				"statuses":[
					"success",
					{"resting":{"oid":42}},
					{"error":"insufficient margin"}
				]
			}
		},
		"unexpected":"value"
	}`

	var resp ExchangeResponse
	require.NoError(t, json.Unmarshal([]byte(payload), &resp))
	require.Equal(t, "ok", resp.Status)
	require.Equal(t, "0xtxhash", resp.TxHash)
	require.NotNil(t, resp.Response)
	require.Len(t, resp.Response.Data.Statuses, 3)

	first := resp.Response.Data.Statuses[0]
	require.Equal(t, ExchangeStatusSuccess, first.Kind)
	require.True(t, first.Success)

	second := resp.Response.Data.Statuses[1]
	require.Equal(t, ExchangeStatusResting, second.Kind)
	require.NotNil(t, second.Resting)
	require.Equal(t, int64(42), second.Resting.OrderID)

	third := resp.Response.Data.Statuses[2]
	require.Equal(t, ExchangeStatusError, third.Kind)
	require.Equal(t, "insufficient margin", third.Error)

	require.Len(t, resp.Extras, 1)
	require.Equal(t, "value", resp.Extras["unexpected"])
}

func TestSpotDeployAddressWeiMarshal(t *testing.T) {
	value := SpotDeployAddressWei{Address: "0xabc", Wei: "10"}
	data, err := json.Marshal(value)
	require.NoError(t, err)
	require.JSONEq(t, `["0xabc","10"]`, string(data))
}

func TestSendAssetActionMarshal(t *testing.T) {
	action := SendAssetAction{
		Type:           "sendAsset",
		Destination:    "0xdest",
		SourceDEX:      "perp",
		DestinationDEX: "spot",
		Token:          "USDC",
		Amount:         "1.25",
		FromSubAccount: "",
	}
	data, err := json.Marshal(action)
	require.NoError(t, err)
	require.JSONEq(t, `{"type":"sendAsset","destination":"0xdest","sourceDex":"perp","destinationDex":"spot","token":"USDC","amount":"1.25","fromSubAccount":""}`, string(data))
}

func TestExchangeResponseExtractOrderStatus(t *testing.T) {
	resp := &ExchangeResponse{
		Status: "ok",
		Response: &ExchangeResponseBody{
			Type: "order",
			Data: ExchangeResponseData{
				Statuses: []ExchangeStatusEntry{
					{Kind: ExchangeStatusSuccess, Success: true},
					{Kind: ExchangeStatusResting, Resting: &ExchangeOrderState{OrderID: 42}},
				},
			},
		},
	}
	orderID, status, subErr, err := resp.ExtractOrderStatus()
	require.NoError(t, err)
	require.NoError(t, subErr)
	require.Equal(t, "42", orderID)
	require.Equal(t, order.Active, status)

	resp.Response.Data.Statuses = append(resp.Response.Data.Statuses, ExchangeStatusEntry{Kind: ExchangeStatusError, Error: "insufficient margin"})
	_, _, subErr, err = resp.ExtractOrderStatus()
	require.NoError(t, err)
	require.ErrorIs(t, subErr, errExchangeStatusEntryError)
	require.ErrorContains(t, subErr, "insufficient margin")
}
