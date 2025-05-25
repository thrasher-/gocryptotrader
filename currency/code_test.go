package currency

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
)

func TestRoleString(t *testing.T) {
	if Unset.String() != UnsetRoleString {
		t.Errorf("Role String() error expected %s but received %s",
			UnsetRoleString,
			Unset)
	}

	if Fiat.String() != FiatCurrencyString {
		t.Errorf("Role String() error expected %s but received %s",
			FiatCurrencyString,
			Fiat)
	}

	if Cryptocurrency.String() != CryptocurrencyString {
		t.Errorf("Role String() error expected %s but received %s",
			CryptocurrencyString,
			Cryptocurrency)
	}

	if Token.String() != TokenString {
		t.Errorf("Role String() error expected %s but received %s",
			TokenString,
			Token)
	}

	if Contract.String() != ContractString {
		t.Errorf("Role String() error expected %s but received %s",
			ContractString,
			Contract)
	}

	var random Role = 1 << 7

	if random.String() != UnsetRoleString {
		t.Errorf("Role String() error expected %s but received %s",
			"UNKNOWN",
			random)
	}
}

func TestRoleMarshalJSON(t *testing.T) {
	d, err := json.Marshal(Fiat)
	assert.NoError(t, err, "Role MarshalJSON() error")

	if expected := `"fiatcurrency"`; string(d) != expected {
		t.Errorf("Role MarshalJSON() error expected %s but received %s",
			expected,
			string(d))
	}
}

// TestRoleUnmarshalJSON logic test
func TestRoleUnmarshalJSON(t *testing.T) {
	type AllTheRoles struct {
		RoleOne     Role `json:"RoleOne"`
		RoleTwo     Role `json:"RoleTwo"`
		RoleThree   Role `json:"RoleThree"`
		RoleFour    Role `json:"RoleFour"`
		RoleFive    Role `json:"RoleFive"`
		RoleSix     Role `json:"RoleSix"`
		RoleUnknown Role `json:"RoleUnknown"`
	}

	outgoing := AllTheRoles{
		RoleOne:   Unset,
		RoleTwo:   Cryptocurrency,
		RoleThree: Fiat,
		RoleFour:  Token,
		RoleFive:  Contract,
		RoleSix:   Stable,
	}

	e, err := json.Marshal(1337)
	require.NoError(t, err, "Role UnmarshalJSON() error marshalling 1337")

	var incoming AllTheRoles
	err = json.Unmarshal(e, &incoming)
	assert.Error(t, err, "Role UnmarshalJSON() Expected error when unmarshalling 1337 to AllTheRoles")

	e, err = json.Marshal(outgoing)
	require.NoError(t, err, "Role UnmarshalJSON() error marshalling outgoing")

	err = json.Unmarshal(e, &incoming)
	require.NoError(t, err, "Role UnmarshalJSON() error unmarshalling to incoming")

	if incoming.RoleOne != Unset {
		t.Errorf("Role String() error expected %s but received %s",
			Unset,
			incoming.RoleOne)
	}

	if incoming.RoleTwo != Cryptocurrency {
		t.Errorf("Role String() error expected %s but received %s",
			Cryptocurrency,
			incoming.RoleTwo)
	}

	if incoming.RoleThree != Fiat {
		t.Errorf("Role String() error expected %s but received %s",
			Fiat,
			incoming.RoleThree)
	}

	if incoming.RoleFour != Token {
		t.Errorf("Role String() error expected %s but received %s",
			Token,
			incoming.RoleFour)
	}

	if incoming.RoleFive != Contract {
		t.Errorf("Role String() error expected %s but received %s",
			Contract,
			incoming.RoleFive)
	}

	if incoming.RoleUnknown != Unset {
		t.Errorf("Role String() error expected %s but received %s",
			incoming.RoleFive,
			incoming.RoleUnknown)
	}
	var unhandled Role
	err = unhandled.UnmarshalJSON([]byte("\"ThisIsntReal\""))
	assert.Error(t, err, "Expected unmarshall error for ThisIsntReal")

	err = unhandled.UnmarshalJSON([]byte(`1336`))
	assert.Error(t, err, "Expected unmarshall error for 1336")
}

func (b *BaseCodes) assertRole(t *testing.T, c Code, r Role) {
	t.Helper()
	b.mtx.Lock()
	defer b.mtx.Unlock()
	stored, ok := b.Items[c.Item.Symbol]
	if !ok {
		t.Fatal("code pointer not found")
	}
	for x := range stored {
		if stored[x] != c.Item {
			continue
		}
		if stored[x].Role != r {
			t.Fatalf("unexpected role received: %v but expected: %v", stored[x].Role, r)
		}
		return
	}
	t.Fatal("code pointer not found")
}

func TestBaseCode(t *testing.T) {
	var main BaseCodes
	if main.HasData() {
		t.Errorf("BaseCode HasData() error expected false but received %v",
			main.HasData())
	}

	catsUnset := main.Register("CATS", Unset)
	main.assertRole(t, catsUnset, Unset)
	if !main.HasData() {
		t.Errorf("BaseCode HasData() error expected true but received %v",
			main.HasData())
	}

	// Changes unset to fiat
	catsFiat := main.Register("CATS", Fiat)
	main.assertRole(t, catsUnset, Fiat)

	// Register as unset, will return first match.
	otherFiatCat := main.Register("CATS", Unset)
	main.assertRole(t, otherFiatCat, Fiat)
	if !otherFiatCat.Equal(catsFiat) {
		t.Errorf("BaseCode Match() error expected true but received %v",
			false)
	}

	// Register as fiat, will return fiat match.
	thatOtherFiatCat := main.Register("CATS", Fiat)
	main.assertRole(t, otherFiatCat, Fiat)
	if !thatOtherFiatCat.Equal(catsFiat) {
		t.Errorf("BaseCode Match() error expected true but received %v",
			false)
	}

	// Register as stable, will return a different currency with the same
	// currency code.
	superStableCatNoShakes := main.Register("CATS", Stable)
	main.assertRole(t, superStableCatNoShakes, Stable)
	if superStableCatNoShakes.Equal(catsFiat) {
		t.Errorf("BaseCode Match() error expected true but received %v",
			true)
	}

	// Due to the role being unset originally, this will be set to Fiat when
	// explicitly set.
	if !catsUnset.Equal(catsFiat) {
		t.Fatal("both should be the same")
	}

	if main.Register("DOGS", Unset).Equal(catsUnset) {
		t.Errorf("BaseCode Match() error expected false but received %v",
			true)
	}

	loadedCurrencies := main.GetCurrencies()

	if loadedCurrencies.Contains(main.Register("OWLS", Unset)) {
		t.Errorf("BaseCode Contains() error expected false but received %v",
			true)
	}

	if !loadedCurrencies.Contains(catsFiat) {
		t.Errorf("BaseCode Contains() error expected true but received %v",
			false)
	}

	main.Register("XBTUSD", Unset)
	err := main.UpdateCurrency(&Item{
		FullName: "Bitcoin Perpetual",
		Symbol:   "XBTUSD",
		Role:     Contract,
	})
	require.NoError(t, err)

	main.Register("BTC", Unset)
	err = main.UpdateCurrency(&Item{
		FullName: "Bitcoin",
		Symbol:   "BTC",
		ID:       1337,
	})
	require.ErrorIs(t, err, errRoleUnset)

	err = main.UpdateCurrency(&Item{
		FullName: "Bitcoin",
		Symbol:   "BTC",
		ID:       1337,
		Role:     Cryptocurrency,
	})
	require.NoError(t, err)

	aud := main.Register("AUD", Unset)
	err = main.UpdateCurrency(&Item{
		FullName: "Unreal Dollar",
		Symbol:   "AUD",
		ID:       1111,
		Role:     Fiat,
	})
	require.NoError(t, err)

	if aud.Item.FullName != "Unreal Dollar" {
		t.Error("Expected fullname to update for AUD")
	}

	err = main.UpdateCurrency(&Item{
		FullName: "Australian Dollar",
		Symbol:   "AUD",
		ID:       1336,
		Role:     Fiat,
	})
	require.NoError(t, err)

	aud.Item.Role = Unset
	err = main.UpdateCurrency(&Item{
		FullName: "Australian Dollar",
		Symbol:   "AUD",
		ID:       1336,
		Role:     Fiat,
	})
	require.NoError(t, err)
	if aud.Item.Role != Fiat {
		t.Error("Expected role to change to Fiat")
	}

	main.Register("PPT", Unset)
	err = main.UpdateCurrency(&Item{
		FullName:   "Populous",
		Symbol:     "PPT",
		AssocChain: "ETH",
		ID:         1335,
		Role:       Token,
	})
	require.NoError(t, err)

	contract := main.Register("XBTUSD", Unset)

	if contract.IsFiatCurrency() {
		t.Errorf("BaseCode IsFiatCurrency() error expected false but received %v",
			true)
	}

	if contract.IsCryptocurrency() {
		t.Errorf("BaseCode IsCryptocurrency() error expected false but received %v",
			true)
	}

	err = main.LoadItem(nil)
	require.ErrorIs(t, err, errItemIsNil)

	err = main.LoadItem(&Item{})
	require.ErrorIs(t, err, errItemIsEmpty)

	err = main.LoadItem(&Item{
		ID:       0,
		FullName: "Cardano",
		Role:     Cryptocurrency,
		Symbol:   "ADA",
	})
	require.NoError(t, err)

	err = main.LoadItem(&Item{
		ID:       0,
		FullName: "Cardano",
		Role:     Cryptocurrency,
		Symbol:   "ADA",
	})
	require.NoError(t, err)

	full, err := main.GetFullCurrencyData()
	assert.NoError(t, err, "BaseCode GetFullCurrencyData error")

	if len(full.Contracts) != 1 {
		t.Errorf("BaseCode GetFullCurrencyData() error expected 1 but received %v",
			len(full.Contracts))
	}

	if len(full.Cryptocurrency) != 2 {
		t.Errorf("BaseCode GetFullCurrencyData() error expected 1 but received %v",
			len(full.Cryptocurrency))
	}

	if len(full.FiatCurrency) != 2 {
		t.Errorf("BaseCode GetFullCurrencyData() error expected 1 but received %v",
			len(full.FiatCurrency))
	}

	if len(full.Token) != 1 {
		t.Errorf("BaseCode GetFullCurrencyData() error expected 1 but received %v",
			len(full.Token))
	}

	if len(full.UnsetCurrency) != 2 {
		t.Errorf("BaseCode GetFullCurrencyData() error expected 3 but received %v",
			len(full.UnsetCurrency))
	}

	lastMainUpdate, ok := full.LastMainUpdate.(int64)
	if !ok {
		t.Error("unable to type assert LastMainUpdate")
	}
	if lastMainUpdate != -62135596800 {
		t.Errorf("BaseCode GetFullCurrencyData() error expected -62135596800 but received %d",
			full.LastMainUpdate)
	}

	err = main.LoadItem(&Item{
		ID:       0,
		FullName: "Cardano",
		Role:     Role(99),
		Symbol:   "ADA",
	})
	assert.NoError(t, err, "BaseCode LoadItem() error") // This was t.Error, assuming NoError is expected unless message means something else
	_, err = main.GetFullCurrencyData()
	assert.Error(t, err, "Expected 'Role undefined'")

	main.Items["CATS"][0].FullName = "Hello"
	err = main.UpdateCurrency(&Item{
		FullName: "MEWOW",
		Symbol:   "CATS",
		ID:       1338,
		Role:     Fiat,
	})
	require.NoError(t, err)

	if main.Items["CATS"][0].FullName != "MEWOW" {
		t.Error("Fullname not updated")
	}

	err = main.UpdateCurrency(&Item{
		FullName: "WOWCATS",
		Symbol:   "CATS",
		ID:       3,
		Role:     Fiat,
	})
	require.NoError(t, err)

	// Creates a new item under a different currency role
	if main.Items["CATS"][0].ID != 3 {
		t.Error("ID not updated")
	}

	main.Items["CATS"][0].Role = Unset

	err = main.UpdateCurrency(&Item{
		FullName: "MEWOW",
		Symbol:   "CATS",
		ID:       1338,
		Role:     Cryptocurrency,
	})
	require.NoError(t, err)
	if main.Items["CATS"][0].ID != 1338 {
		t.Error("ID not updated")
	}
}

func TestNewCodeFormatting(t *testing.T) {
	require.True(t, NewCode("BTC").upperCase)
	require.False(t, NewCode("btc").upperCase)
	require.True(t, NewCode("BTC").Equal(NewCode("btc")))
	require.False(t, NewCode("420").upperCase)
	require.False(t, NewCode("btc420").upperCase)
	require.False(t, NewCode("420").Lower().upperCase)
	require.True(t, NewCode("4BTC").upperCase)
}

func TestCodeString(t *testing.T) {
	if cc, expected := NewCode("TEST"), "TEST"; cc.String() != expected {
		t.Errorf("Currency Code String() error expected %s but received %s",
			expected, cc)
	}
}

func TestCodeLower(t *testing.T) {
	if cc, expected := NewCode("TEST"), "test"; cc.Lower().String() != expected {
		t.Errorf("Currency Code Lower() error expected %s but received %s",
			expected,
			cc.Lower())
	}
}

func TestCodeUpper(t *testing.T) {
	if cc, expected := NewCode("test"), "TEST"; cc.Upper().String() != expected {
		t.Errorf("Currency Code Upper() error expected %s but received %s",
			expected,
			cc.Upper())
	}
}

func TestCodeUnmarshalJSON(t *testing.T) {
	var unmarshalHere Code
	expected := "BRO"
	encoded, err := json.Marshal(expected)
	require.NoError(t, err, "Currency Code UnmarshalJSON error marshalling expected")

	err = json.Unmarshal(encoded, &unmarshalHere)
	require.NoError(t, err, "Currency Code UnmarshalJSON error unmarshalling to unmarshalHere first time")

	// Repeating unmarshal to the same variable, assuming this was intentional for some test reason.
	err = json.Unmarshal(encoded, &unmarshalHere)
	require.NoError(t, err, "Currency Code UnmarshalJSON error unmarshalling to unmarshalHere second time")

	if unmarshalHere.String() != expected {
		t.Errorf("Currency Code Upper() error expected %s but received %s",
			expected,
			unmarshalHere)
	}

	encoded, err = json.Marshal(1336) // :'(
	require.NoError(t, err, "Currency Code UnmarshalJSON error marshalling 1336")
	err = json.Unmarshal(encoded, &unmarshalHere)
	assert.Error(t, err, "expected error when unmarshalling int to Code")
}

func TestCodeMarshalJSON(t *testing.T) {
	quickstruct := struct {
		Codey Code `json:"sweetCodes"`
	}{
		Codey: NewCode("BRO"),
	}

	expectedJSON := `{"sweetCodes":"BRO"}`

	encoded, err := json.Marshal(quickstruct)
	require.NoError(t, err, "Currency Code MarshalJSON error for normal struct")

	if string(encoded) != expectedJSON {
		t.Errorf("Currency Code Upper() error expected %s but received %s",
			expectedJSON,
			string(encoded))
	}

	quickstruct = struct {
		Codey Code `json:"sweetCodes"`
	}{
		Codey: EMPTYCODE, // nil code
	}

	encoded, err = json.Marshal(quickstruct)
	require.NoError(t, err, "Currency Code MarshalJSON error for struct with EMPTYCODE")

	newExpectedJSON := `{"sweetCodes":""}`
	if string(encoded) != newExpectedJSON {
		t.Errorf("Currency Code Upper() error expected %s but received %s",
			newExpectedJSON, string(encoded))
	}
}

func TestIsFiatCurrency(t *testing.T) {
	if EMPTYCODE.IsFiatCurrency() {
		t.Errorf("TestIsFiatCurrency cannot match currency, %s.", EMPTYCODE)
	}
	if !USD.IsFiatCurrency() {
		t.Errorf("TestIsFiatCurrency cannot match currency, %s.", USD)
	}
	if !CNY.IsFiatCurrency() {
		t.Errorf("TestIsFiatCurrency cannot match currency, %s.", CNY)
	}
	if LINO.IsFiatCurrency() {
		t.Errorf("TestIsFiatCurrency cannot match currency, %s.", LINO)
	}
	if USDT.IsFiatCurrency() {
		t.Errorf("TestIsFiatCurrency cannot match currency, %s.", USDT)
	}
	if DAI.IsFiatCurrency() {
		t.Errorf("TestIsFiatCurrency cannot match currency, %s.", DAI)
	}
}

func TestIsCryptocurrency(t *testing.T) {
	if EMPTYCODE.IsCryptocurrency() {
		t.Errorf("TestIsCryptocurrency cannot match currency, %s.",
			EMPTYCODE)
	}
	if !BTC.IsCryptocurrency() {
		t.Errorf("TestIsCryptocurrency cannot match currency, %s.",
			BTC)
	}
	if !LTC.IsCryptocurrency() {
		t.Errorf("TestIsCryptocurrency cannot match currency, %s.",
			LTC)
	}
	if AUD.IsCryptocurrency() {
		t.Errorf("TestIsCryptocurrency cannot match currency, %s.",
			AUD)
	}
	if !USDT.IsCryptocurrency() {
		t.Errorf(
			"TestIsCryptocurrency cannot match currency, %s.", USD)
	}
	if !DAI.IsCryptocurrency() {
		t.Errorf(
			"TestIsCryptocurrency cannot match currency, %s.", USD)
	}
}

func TestIsStableCurrency(t *testing.T) {
	if EMPTYCODE.IsStableCurrency() {
		t.Errorf("TestIsStableCurrency cannot match currency, %s.", EMPTYCODE)
	}
	if BTC.IsStableCurrency() {
		t.Errorf("TestIsStableCurrency cannot match currency, %s.", BTC)
	}
	if LTC.IsStableCurrency() {
		t.Errorf("TestIsStableCurrency cannot match currency, %s.", LTC)
	}
	if AUD.IsStableCurrency() {
		t.Errorf("TestIsStableCurrency cannot match currency, %s.", AUD)
	}
	if !USDT.IsStableCurrency() {
		t.Errorf("TestIsStableCurrency cannot match currency, %s.", USDT)
	}
	if !DAI.IsStableCurrency() {
		t.Errorf("TestIsStableCurrency cannot match currency, %s.", DAI)
	}
}

func TestItemString(t *testing.T) {
	newItem := Item{
		ID:         1337,
		FullName:   "Hello,World",
		Symbol:     "HWORLD",
		AssocChain: "Silly",
	}

	assert.Equal(t, "HWORLD", newItem.String())
}

// 28848025	        40.84 ns/op	       8 B/op	       1 allocs/op // Current
//
//	546290	      2192 ns/op	       8 B/op	       1 allocs/op // Previous
func BenchmarkNewCode(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_ = NewCode("someCode")
	}
}
