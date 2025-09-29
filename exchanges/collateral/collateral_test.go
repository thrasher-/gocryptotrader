package collateral

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
)

func TestValidCollateralType(t *testing.T) {
	t.Parallel()
	assert.True(t, SingleMode.Valid(), "SingleMode.Valid should return true")
	assert.True(t, MultiMode.Valid(), "MultiMode.Valid should return true")
	assert.True(t, PortfolioMode.Valid(), "PortfolioMode.Valid should return true")
	assert.True(t, SpotFuturesMode.Valid(), "SpotFuturesMode.Valid should return true")
	assert.False(t, UnsetMode.Valid(), "UnsetMode.Valid should return false")
	assert.False(t, UnknownMode.Valid(), "UnknownMode.Valid should return false")
	assert.False(t, Mode(137).Valid(), "Mode.Valid should return false for undefined input")
}

func TestUnmarshalJSONCollateralType(t *testing.T) {
	t.Parallel()
	type martian struct {
		M Mode `json:"collateral"`
	}

	var alien martian
	jason := []byte(`{"collateral":"single"}`)
	require.NoError(t, json.Unmarshal(jason, &alien))
	assert.Equal(t, SingleMode, alien.M, "Unmarshal should yield SingleMode")

	jason = []byte(`{"collateral":"multi"}`)
	require.NoError(t, json.Unmarshal(jason, &alien))
	assert.Equal(t, MultiMode, alien.M, "Unmarshal should yield MultiMode")

	jason = []byte(`{"collateral":"portfolio"}`)
	require.NoError(t, json.Unmarshal(jason, &alien))
	assert.Equal(t, PortfolioMode, alien.M, "Unmarshal should yield PortfolioMode")

	jason = []byte(`{"collateral":"hello moto"}`)
	err := json.Unmarshal(jason, &alien)
	assert.ErrorIs(t, err, ErrInvalidCollateralMode)

	assert.Equal(t, UnknownMode, alien.M, "Unmarshal error should set UnknownMode")
}

func TestStringCollateralType(t *testing.T) {
	t.Parallel()
	assert.Equal(t, unknownCollateralStr, UnknownMode.String(), "UnknownMode string should match")
	assert.Equal(t, singleCollateralStr, SingleMode.String(), "SingleMode string should match")
	assert.Equal(t, multiCollateralStr, MultiMode.String(), "MultiMode string should match")
	assert.Equal(t, portfolioCollateralStr, PortfolioMode.String(), "PortfolioMode string should match")
	assert.Equal(t, unsetCollateralStr, UnsetMode.String(), "UnsetMode string should match")
}

func TestUpperCollateralType(t *testing.T) {
	t.Parallel()
	assert.Equal(t, strings.ToUpper(unknownCollateralStr), UnknownMode.Upper(), "UnknownMode upper should match")
	assert.Equal(t, strings.ToUpper(singleCollateralStr), SingleMode.Upper(), "SingleMode upper should match")
	assert.Equal(t, strings.ToUpper(multiCollateralStr), MultiMode.Upper(), "MultiMode upper should match")
	assert.Equal(t, strings.ToUpper(portfolioCollateralStr), PortfolioMode.Upper(), "PortfolioMode upper should match")
	assert.Equal(t, strings.ToUpper(unsetCollateralStr), UnsetMode.Upper(), "UnsetMode upper should match")
}

func TestIsValidCollateralTypeString(t *testing.T) {
	t.Parallel()
	assert.False(t, IsValidCollateralModeString("lol"), "IsValidCollateralModeString should return false for invalid input")
	assert.True(t, IsValidCollateralModeString("single"), "IsValidCollateralModeString should return true for single")
	assert.True(t, IsValidCollateralModeString("multi"), "IsValidCollateralModeString should return true for multi")
	assert.True(t, IsValidCollateralModeString("portfolio"), "IsValidCollateralModeString should return true for portfolio")
	assert.True(t, IsValidCollateralModeString("unset"), "IsValidCollateralModeString should return true for unset")
	assert.False(t, IsValidCollateralModeString(""), "IsValidCollateralModeString should return false for empty input")
	assert.False(t, IsValidCollateralModeString("unknown"), "IsValidCollateralModeString should return false for unknown")
}

func TestStringToCollateralType(t *testing.T) {
	t.Parallel()
	resp, err := StringToMode("lol")
	assert.ErrorIs(t, err, ErrInvalidCollateralMode)
	assert.Equal(t, UnknownMode, resp, "Invalid mode should return UnknownMode")

	resp, err = StringToMode("")
	require.NoError(t, err)
	assert.Equal(t, UnsetMode, resp, "Empty string should return UnsetMode")

	resp, err = StringToMode("single")
	require.NoError(t, err)
	assert.Equal(t, SingleMode, resp, `"single" should return SingleMode`)

	resp, err = StringToMode("multi")
	require.NoError(t, err)
	assert.Equal(t, MultiMode, resp, `"multi" should return MultiMode`)

	resp, err = StringToMode("portfolio")
	require.NoError(t, err)
	assert.Equal(t, PortfolioMode, resp, `"portfolio" should return PortfolioMode`)
}
