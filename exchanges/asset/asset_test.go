package asset

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
)

func TestString(t *testing.T) {
	t.Parallel()
	for a := range All {
		if a == 0 {
			assert.Empty(t, a.String(), "Empty.String should return empty")
		} else {
			assert.NotEmptyf(t, a.String(), "%s.String should return empty", a)
		}
	}
}

func TestUpper(t *testing.T) {
	t.Parallel()
	a := Spot
	require.Equal(t, "SPOT", a.Upper())
	a = 0
	require.Empty(t, a.Upper())
}

func TestStrings(t *testing.T) {
	t.Parallel()
	assert.ElementsMatch(t, Items{Spot, Futures}.Strings(), []string{"spot", "futures"})
}

func TestContains(t *testing.T) {
	t.Parallel()
	a := Items{Spot, Futures}
	assert.False(t, a.Contains(666), "Items.Contains should return false for unsupported asset")
	assert.True(t, a.Contains(Spot), "Items.Contains should return true for spot")
	assert.False(t, a.Contains(Binary), "Items.Contains should return false for binary when not in list")
	assert.False(t, a.Contains(0), "Items.Contains should return false for empty asset")
}

func TestJoinToString(t *testing.T) {
	t.Parallel()
	a := Items{Spot, Futures}
	assert.Equal(t, "spot,futures", a.JoinToString(","), "Items.JoinToString should join assets with delimiter")
}

func TestIsValid(t *testing.T) {
	t.Parallel()
	for a := range All {
		if a.String() == "" {
			require.Falsef(t, a.IsValid(), "IsValid must return false with non-asset value %d", a)
		} else {
			require.Truef(t, a.IsValid(), "IsValid must return true for %s", a)
		}
	}
	require.False(t, All.IsValid(), "IsValid must return false for All")
}

func TestIsFutures(t *testing.T) {
	t.Parallel()
	valid := []Item{PerpetualContract, PerpetualSwap, Futures, DeliveryFutures, UpsideProfitContract, DownsideProfitContract, CoinMarginedFutures, USDTMarginedFutures, USDCMarginedFutures, FutureCombo, LinearContract, Spread}
	for a := range All {
		if slices.Contains(valid, a) {
			require.Truef(t, a.IsFutures(), "IsFutures must return true for %s", a)
		} else {
			require.Falsef(t, a.IsFutures(), "IsFutures must return false for non-asset value %d (%s)", a, a)
		}
	}
}

func TestIsOptions(t *testing.T) {
	t.Parallel()
	valid := []Item{Options, OptionCombo}
	for a := range All {
		if slices.Contains(valid, a) {
			require.Truef(t, a.IsOptions(), "IsOptions must return true for %s", a)
		} else {
			require.Falsef(t, a.IsOptions(), "IsOptions must return false for non-asset value %d (%s)", a, a)
		}
	}
}

func TestNew(t *testing.T) {
	t.Parallel()
	cases := []struct {
		Input    string
		Expected Item
		Error    error
	}{
		{Input: "Spota", Error: ErrNotSupported},
		{Input: "MARGIN", Expected: Margin},
		{Input: "MARGINFUNDING", Expected: MarginFunding},
		{Input: "INDEX", Expected: Index},
		{Input: "BINARY", Expected: Binary},
		{Input: "PERPETUALCONTRACT", Expected: PerpetualContract},
		{Input: "PERPETUALSWAP", Expected: PerpetualSwap},
		{Input: "FUTURES", Expected: Futures},
		{Input: "UpsideProfitContract", Expected: UpsideProfitContract},
		{Input: "DownsideProfitContract", Expected: DownsideProfitContract},
		{Input: "CoinMarginedFutures", Expected: CoinMarginedFutures},
		{Input: "USDTMarginedFutures", Expected: USDTMarginedFutures},
		{Input: "USDCMarginedFutures", Expected: USDCMarginedFutures},
		{Input: "Options", Expected: Options},
		{Input: "Option", Expected: Options},
		{Input: "Future", Error: ErrNotSupported},
		{Input: "option_combo", Expected: OptionCombo},
		{Input: "future_combo", Expected: FutureCombo},
		{Input: "spread", Expected: Spread},
		{Input: "linearContract", Expected: LinearContract},
	}

	for _, tt := range cases {
		t.Run("", func(t *testing.T) {
			t.Parallel()
			returned, err := New(tt.Input)
			require.ErrorIs(t, err, tt.Error)
			assert.Equalf(t, tt.Expected, returned, "New should return expected asset for %s", tt.Input)
		})
	}
}

func TestSupported(t *testing.T) {
	t.Parallel()
	s := Supported()
	assert.Equal(t, len(supportedList), len(s), "Supported should return list matching supportedList length")
	for i := range supportedList {
		assert.Equal(t, supportedList[i], s[i], "Supported should return assets in expected order")
	}
}

func TestUnmarshalMarshal(t *testing.T) {
	t.Parallel()
	data, err := json.Marshal(Item(0))
	require.NoError(t, err)

	assert.Equal(t, `""`, string(data), "Marshal should encode empty asset as empty string")

	data, err = json.Marshal(Spot)
	require.NoError(t, err)

	assert.Equal(t, `"spot"`, string(data), "Marshal should encode spot asset to string")

	var spot Item

	err = json.Unmarshal(data, &spot)
	require.NoError(t, err)

	assert.Equal(t, Spot, spot, "Unmarshal should decode spot asset")

	err = json.Unmarshal([]byte(`"confused"`), &spot)
	require.ErrorIs(t, err, ErrNotSupported)

	err = json.Unmarshal([]byte(`""`), &spot)
	require.NoError(t, err)

	err = json.Unmarshal([]byte(`123`), &spot)
	assert.Error(t, err, "Unmarshal should error correctly")
}

func TestUseDefault(t *testing.T) {
	t.Parallel()
	assert.Equal(t, Spot, UseDefault(), "UseDefault should return spot asset")
}
