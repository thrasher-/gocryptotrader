package currency

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
)

const (
	defaultPair           = "BTCUSD"
	defaultPairWDelimiter = "BTC-USD"
)

func TestLower(t *testing.T) {
	t.Parallel()
	pair, err := NewPairFromString(defaultPair)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := NewPairFromString(defaultPair)
	if err != nil {
		t.Fatal(err)
	}
	if actual := pair.Lower(); actual.String() != expected.Lower().String() {
		t.Errorf("Lower(): %s was not equal to expected value: %s",
			actual,
			expected.Lower())
	}
}

func TestUpper(t *testing.T) {
	t.Parallel()
	pair, err := NewPairFromString(defaultPair)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := NewPairFromString(defaultPair)
	if err != nil {
		t.Fatal(err)
	}
	if actual := pair.Upper(); actual.String() != expected.String() {
		t.Errorf("Upper(): %s was not equal to expected value: %s",
			actual, expected)
	}
}

func TestPairUnmarshalJSON(t *testing.T) {
	var p Pair
	assert.NoError(t, p.UnmarshalJSON([]byte(`"btc_usd"`)), "UnmarshalJSON should not error")
	assert.Equal(t, "btc", p.Base.String(), "Base should be correct")
	assert.Equal(t, "usd", p.Quote.String(), "Quote should be correct")
	assert.Equal(t, "_", p.Delimiter, "Delimiter should be correct")

	assert.ErrorIs(t, p.UnmarshalJSON([]byte(`"btcusd"`)), ErrCreatingPair, "UnmarshalJSON with no delimiter should error")

	assert.NoError(t, p.UnmarshalJSON([]byte(`""`)), "UnmarshalJSON should not error on empty value")
	assert.Equal(t, EMPTYPAIR, p, "UnmarshalJSON empty value should give EMPTYPAIR")
	assert.NoError(t, p.UnmarshalJSON([]byte(`null`)), "UnmarshalJSON should not error on empty value")
	assert.Equal(t, EMPTYPAIR, p, "UnmarshalJSON null value should give EMPTYPAIR")
}

func TestPairMarshalJSON(t *testing.T) {
	quickstruct := struct {
		Pair *Pair `json:"superPair"`
	}{
		&Pair{Base: BTC, Quote: USD, Delimiter: "-"},
	}

	encoded, err := json.Marshal(quickstruct)
	if err != nil {
		t.Fatal("Pair MarshalJSON() error", err)
	}

	expected := `{"superPair":"BTC-USD"}`
	if string(encoded) != expected {
		t.Errorf("Pair MarshalJSON() error expected %s but received %s",
			expected, string(encoded))
	}
}

func TestIsCryptoPair(t *testing.T) {
	if !NewPair(BTC, LTC).IsCryptoPair() {
		t.Error("TestIsCryptoPair. Expected true result")
	}

	if NewBTCUSD().IsCryptoPair() {
		t.Error("TestIsCryptoPair. Expected false result")
	}
}

func TestIsCryptoFiatPair(t *testing.T) {
	if !NewBTCUSD().IsCryptoFiatPair() {
		t.Error("TestIsCryptoPair. Expected true result")
	}

	if NewPair(BTC, LTC).IsCryptoFiatPair() {
		t.Error("TestIsCryptoPair. Expected false result")
	}
}

func TestIsFiatPair(t *testing.T) {
	if !NewPair(AUD, USD).IsFiatPair() {
		t.Error("TestIsFiatPair. Expected true result")
	}

	if NewPair(BTC, AUD).IsFiatPair() {
		t.Error("TestIsFiatPair. Expected false result")
	}
}

func TestIsCryptoStablePair(t *testing.T) {
	if !NewBTCUSDT().IsCryptoStablePair() {
		t.Error("TestIsCryptoStablePair. Expected true result")
	}

	if !NewPair(DAI, USDT).IsCryptoStablePair() {
		t.Error("TestIsCryptoStablePair. Expected true result")
	}

	if NewPair(AUD, USDT).IsCryptoStablePair() {
		t.Error("TestIsCryptoStablePair. Expected false result")
	}
}

func TestIsStablePair(t *testing.T) {
	if !NewPair(USDT, DAI).IsStablePair() {
		t.Error("TestIsStablePair. Expected true result")
	}

	if NewPair(USDT, AUD).IsStablePair() {
		t.Error("TestIsStablePair. Expected false result")
	}

	if NewPair(USDT, LTC).IsStablePair() {
		t.Error("TestIsStablePair. Expected false result")
	}
}

func TestString(t *testing.T) {
	t.Parallel()
	pair := NewBTCUSD()
	if actual, expected := defaultPair, pair.String(); actual != expected {
		t.Errorf("String(): %s was not equal to expected value: %s",
			actual, expected)
	}
}

func TestFirstCurrency(t *testing.T) {
	t.Parallel()
	pair := NewBTCUSD()
	if actual, expected := pair.Base, BTC; !actual.Equal(expected) {
		t.Errorf(
			"GetFirstCurrency(): %s was not equal to expected value: %s",
			actual, expected,
		)
	}
}

func TestSecondCurrency(t *testing.T) {
	t.Parallel()
	pair := NewBTCUSD()
	if actual, expected := pair.Quote, USD; !actual.Equal(expected) {
		t.Errorf(
			"GetSecondCurrency(): %s was not equal to expected value: %s",
			actual, expected,
		)
	}
}

func TestPair(t *testing.T) {
	t.Parallel()
	pair := NewBTCUSD()
	if actual, expected := pair.String(), defaultPair; actual != expected {
		t.Errorf(
			"Pair(): %s was not equal to expected value: %s",
			actual, expected,
		)
	}
}

func TestDisplay(t *testing.T) {
	t.Parallel()
	_, err := NewPairDelimiter(defaultPairWDelimiter, "wow")
	if err == nil {
		t.Fatal("error cannot be nil")
	}
	pair, err := NewPairDelimiter(defaultPairWDelimiter, "-")
	if err != nil {
		t.Fatal(err)
	}
	actual := pair.String()
	expected := defaultPairWDelimiter
	if actual != expected {
		t.Errorf(
			"Pair(): %s was not equal to expected value: %s",
			actual, expected,
		)
	}

	actual = EMPTYFORMAT.Format(pair)
	expected = "btcusd"
	if actual != expected {
		t.Errorf(
			"Pair(): %s was not equal to expected value: %s",
			actual, expected,
		)
	}

	actual = pair.Format(PairFormat{Delimiter: "~", Uppercase: true}).String()
	expected = "BTC~USD"
	if actual != expected {
		t.Errorf(
			"Pair(): %s was not equal to expected value: %s",
			actual, expected,
		)
	}
}

func TestEquall(t *testing.T) {
	t.Parallel()
	pair := NewBTCUSD()
	secondPair := NewBTCUSD()
	actual := pair.Equal(secondPair)
	expected := true
	if actual != expected {
		t.Errorf(
			"Equal(): %v was not equal to expected value: %v",
			actual, expected,
		)
	}

	secondPair.Quote = ETH
	actual = pair.Equal(secondPair)
	expected = false
	if actual != expected {
		t.Errorf(
			"Equal(): %v was not equal to expected value: %v",
			actual, expected,
		)
	}

	secondPair = NewPair(USD, BTC)
	actual = pair.Equal(secondPair)
	expected = false
	if actual != expected {
		t.Errorf(
			"Equal(): %v was not equal to expected value: %v",
			actual, expected,
		)
	}
}

func TestEqualIncludeReciprocal(t *testing.T) {
	t.Parallel()
	pair := NewBTCUSD()
	secondPair := NewBTCUSD()
	actual := pair.EqualIncludeReciprocal(secondPair)
	expected := true
	if actual != expected {
		t.Errorf(
			"Equal(): %v was not equal to expected value: %v",
			actual, expected,
		)
	}

	secondPair.Quote = ETH
	actual = pair.EqualIncludeReciprocal(secondPair)
	expected = false
	if actual != expected {
		t.Errorf(
			"Equal(): %v was not equal to expected value: %v",
			actual, expected,
		)
	}

	secondPair = NewPair(USD, BTC)
	actual = pair.EqualIncludeReciprocal(secondPair)
	expected = true
	if actual != expected {
		t.Errorf(
			"Equal(): %v was not equal to expected value: %v",
			actual, expected,
		)
	}
}

func TestSwap(t *testing.T) {
	t.Parallel()
	pair := NewBTCUSD()
	if actual, expected := pair.Swap().String(), "USDBTC"; actual != expected {
		t.Errorf(
			"TestSwap: %s was not equal to expected value: %s",
			actual, expected,
		)
	}
}

func TestEmpty(t *testing.T) {
	t.Parallel()
	pair := NewBTCUSD()
	if pair.IsEmpty() {
		t.Error("Empty() returned true when the pair was initialised")
	}

	p := NewPair(NewCode(""), NewCode(""))
	if !p.IsEmpty() {
		t.Error("Empty() returned true when the pair wasn't initialised")
	}
}

func TestNewPair(t *testing.T) {
	t.Parallel()
	pair := NewBTCUSD()
	if expected, actual := defaultPair, pair.String(); actual != expected {
		t.Errorf(
			"Pair(): %s was not equal to expected value: %s",
			actual, expected,
		)
	}
}

func TestNewPairWithDelimiter(t *testing.T) {
	t.Parallel()
	pair := NewPairWithDelimiter("BTC", "USD", "-test-")
	actual := pair.String()
	expected := "BTC-test-USD"
	if actual != expected {
		t.Errorf(
			"Pair(): %s was not equal to expected value: %s",
			actual, expected,
		)
	}

	pair = NewPairWithDelimiter("BTC", "USD", "")
	actual = pair.String()
	expected = defaultPair
	if actual != expected {
		t.Errorf(
			"Pair(): %s was not equal to expected value: %s",
			actual, expected,
		)
	}
}

func TestNewPairDelimiter(t *testing.T) {
	t.Parallel()
	_, err := NewPairDelimiter("", "")
	require.ErrorIs(t, err, errEmptyPairString)

	_, err = NewPairDelimiter("BTC_USD", "")
	require.ErrorIs(t, err, errDelimiterCannotBeEmpty)

	_, err = NewPairDelimiter("BTC_USD", "wow")
	require.ErrorIs(t, err, errDelimiterNotFound)

	_, err = NewPairDelimiter("BTC_USD", " ")
	require.ErrorIs(t, err, errDelimiterNotFound)

	pair, err := NewPairDelimiter(defaultPairWDelimiter, "-")
	require.NoError(t, err)
	assert.Equal(t, defaultPairWDelimiter, pair.String())
	assert.Equal(t, "-", pair.Delimiter)

	pair, err = NewPairDelimiter("BTC-MOVE-0626", "-")
	require.NoError(t, err)
	assert.Equal(t, "BTC-MOVE-0626", pair.String())

	pair, err = NewPairDelimiter("sETH-USDT", "-")
	require.NoError(t, err)
	assert.Equal(t, "SETH-USDT", pair.String(), "If any upper case is found in set this forces the pair to be uppercase")

	pair, err = NewPairDelimiter("BTC::USD", "::")
	require.NoError(t, err)
	assert.Equal(t, "USD", pair.Quote.String(), "a multi-rune delimiter should be consumed whole")

	pair, err = NewPairDelimiter("BTCΔUSD", "Δ")
	require.NoError(t, err)
	assert.Equal(t, "USD", pair.Quote.String(), "a multi-byte delimiter should not leave a partial rune on the quote")
}

func TestNewPairFromString(t *testing.T) {
	t.Parallel()
	pairStr := defaultPairWDelimiter
	pair, err := NewPairFromString(pairStr)
	if err != nil {
		t.Fatal(err)
	}
	actual := pair.String()
	expected := defaultPairWDelimiter
	if actual != expected {
		t.Errorf(
			"Pair(): %s was not equal to expected value: %s",
			actual, expected,
		)
	}

	pairStr = defaultPair
	pair, err = NewPairFromString(pairStr)
	if err != nil {
		t.Fatal(err)
	}
	actual = pair.String()
	expected = defaultPair
	if actual != expected {
		t.Errorf(
			"Pair(): %s was not equal to expected value: %s",
			actual, expected,
		)
	}
	pairMap := map[string]Pair{
		"BTC_USDT-20230630-45000-C": {Base: NewCode("BTC"), Delimiter: UnderscoreDelimiter, Quote: NewCode("USDT-20230630-45000-C")},
		"BTC-USD-221007":            {Base: NewCode("BTC"), Delimiter: DashDelimiter, Quote: NewCode("USD-221007")},
		"IHT_ETH":                   {Base: NewCode("IHT"), Delimiter: UnderscoreDelimiter, Quote: NewCode("ETH")},
		"BTC-USD-220930-30000-P":    {Base: NewCode("BTC"), Delimiter: DashDelimiter, Quote: NewCode("USD-220930-30000-P")},
		"XBTUSDTM":                  {Base: NewCode("XBT"), Delimiter: "", Quote: NewCode("USDTM")},
		"BTC-PERPETUAL":             {Base: NewCode("BTC"), Delimiter: DashDelimiter, Quote: NewCode("PERPETUAL")},
		"SOL-21OCT22-20-C":          {Base: NewCode("SOL"), Delimiter: DashDelimiter, Quote: NewCode("21OCT22-20-C")},
		"SOL-FS-30DEC22_28OCT22":    {Base: NewCode("SOL"), Delimiter: DashDelimiter, Quote: NewCode("FS-30DEC22_28OCT22")},
	}
	for key, expectedPair := range pairMap {
		pair, err = NewPairFromString(key)
		if err != nil {
			t.Fatal(err)
		}
		if !pair.Equal(expectedPair) || pair.Delimiter != expectedPair.Delimiter {
			t.Errorf("Pair(): %s was not equal to expected value: %s", pair.String(), expectedPair.String())
		}
	}
}

func TestNewPairFromFormattedPairs(t *testing.T) {
	t.Parallel()
	p1, err := NewPairDelimiter("BTC-USDT", "-")
	if err != nil {
		t.Fatal(err)
	}
	p2, err := NewPairDelimiter("LTC-USD", "-")
	if err != nil {
		t.Fatal(err)
	}
	pairs := Pairs{
		p1,
		p2,
	}

	p, err := NewPairFromFormattedPairs("BTCUSDT", pairs, PairFormat{
		Uppercase: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if p.String() != "BTC-USDT" {
		t.Error("TestNewPairFromFormattedPairs: Expected currency was not found")
	}

	p, err = NewPairFromFormattedPairs("btcusdt", pairs, PairFormat{Uppercase: false})
	if err != nil {
		t.Fatal(err)
	}

	if p.String() != "BTC-USDT" {
		t.Error("TestNewPairFromFormattedPairs: Expected currency was not found")
	}

	// Now a wrong one, will default to NewPairFromString
	p, err = NewPairFromFormattedPairs("ethusdt", pairs, EMPTYFORMAT)
	if err != nil {
		t.Fatal(err)
	}

	assert.Equal(t, "ethusdt", p.String(), "NewPairFromFormattedPairs should fall back to parsing the string")
	assert.Equal(t, "eth", p.Base.String(), "NewPairFromFormattedPairs should fall back to parsing the base")
}

func TestContainsCurrency(t *testing.T) {
	p := NewBTCUSD()

	if !p.Contains(BTC) {
		t.Error("TestContains: Expected currency was not found")
	}

	if p.Contains(ETH) {
		t.Error("TestContains: Non-existent currency was found")
	}
}

func TestFormatPairs(t *testing.T) {
	_, err := FormatPairs([]string{""}, "-")
	assert.ErrorIs(t, err, errEmptyPairString, "Should error on empty string")

	_, err = FormatPairs([]string{"NO"}, "")
	assert.ErrorIs(t, err, errNoDelimiter, "Should error on a small string with no delimiter")

	newP, err := FormatPairs([]string{defaultPairWDelimiter}, "-")
	assert.NoError(t, err)
	require.NotEmpty(t, newP)
	assert.Equal(t, defaultPairWDelimiter, newP[0].String(), "Pair should format correctly")

	newP, err = FormatPairs([]string{defaultPair}, "")
	assert.NoError(t, err)
	require.NotEmpty(t, newP)
	assert.Equal(t, defaultPair, newP[0].String(), "Pair should format correctly")

	newP, err = FormatPairs([]string{"ETHUSD"}, "")
	assert.NoError(t, err)
	require.NotEmpty(t, newP)
	assert.Equal(t, "ETHUSD", newP[0].String(), "Pair should format correctly")
}

func TestCopyPairFormat(t *testing.T) {
	pairOne := NewBTCUSD()
	pairOne.Delimiter = "-"

	pairs := []Pair{pairOne, NewPair(LTC, USD)}

	testPair := NewBTCUSD()
	testPair.Delimiter = "~"

	result := CopyPairFormat(testPair, pairs, false)
	assert.Equal(t, defaultPairWDelimiter, result.String(), "CopyPairFormat should return correctly formatted pair")

	np := NewPair(ETH, USD)
	result = CopyPairFormat(np, pairs, true)
	assert.Empty(t, result.String(), "CopyPairFormat should return empty string when pair not found")
}

func TestPairsToStringArray(t *testing.T) {
	pairs := Pairs{NewBTCUSD()}

	expected := []string{defaultPair}
	actual := pairs.Strings()

	if actual[0] != expected[0] {
		t.Error("TestPairsToStringArray: Unexpected values")
	}
}

func TestRandomPairFromPairs(t *testing.T) {
	// Test that an empty pairs array returns an empty currency pair
	var emptyPairs Pairs
	result, err := emptyPairs.GetRandomPair()
	require.ErrorIs(t, err, ErrCurrencyPairsEmpty)

	if !result.IsEmpty() {
		t.Error("TestRandomPairFromPairs: Unexpected values")
	}

	// Test that a populated pairs array returns a non-empty currency pair
	pairs := Pairs{NewBTCUSD()} //nolint:prealloc // fixed test fixture, not a hot path
	result, err = pairs.GetRandomPair()
	require.NoError(t, err)

	if result.IsEmpty() {
		t.Error("TestRandomPairFromPairs: Unexpected values")
	}

	// Test that a populated pairs array over a number of attempts returns ALL
	// currency pairs
	pairs = append(pairs, NewPair(ETH, USD))
	expectedResults := make(map[string]bool)
	for range 50 {
		result, err = pairs.GetRandomPair()
		require.NoError(t, err)

		expectedResults[result.String()] = true
	}

	for x := range pairs {
		if !expectedResults[pairs[x].String()] {
			t.Error("TestRandomPairFromPairs: Unexpected values")
		}
	}
}

func TestIsInvalid(t *testing.T) {
	p := NewPair(LTC, LTC)
	if !p.IsInvalid() {
		t.Error("IsInvalid() error expect true but received false")
	}
}

func TestMatchPairsWithNoDelimiter(t *testing.T) {
	p1, err := NewPairDelimiter("BTC-USDT", "-")
	if err != nil {
		t.Fatal(err)
	}
	p2, err := NewPairDelimiter("LTC-USD", "-")
	if err != nil {
		t.Fatal(err)
	}
	pairs := Pairs{p1, p2}
	// Cases append in order to the pairs list that the match cases need
	for _, tc := range []struct{ base, quote string }{
		{base: "EQUAD", quote: "BTC"},
		{base: "HTDF", quote: "USDT"},
		{base: "BETHER", quote: "ETH"},
	} {
		if !t.Run("new pair "+tc.base+"-"+tc.quote, func(t *testing.T) {
			pair, err := NewPairFromStrings(tc.base, tc.quote)
			require.NoError(t, err, "NewPairFromStrings must not error")
			pairs = append(pairs, pair)
		}) {
			t.FailNow()
		}
	}

	// Serial like the parent test
	for _, tc := range []struct {
		name, symbol, delimiter, expectedBase, expectedQuote string
	}{
		{name: "BTCUSDT without delimiter", symbol: "BTCUSDT", expectedBase: "BTC", expectedQuote: "USDT"},
		{name: "EQUADBTC without delimiter", symbol: "EQUADBTC", expectedBase: "EQUAD", expectedQuote: "BTC"},
		{name: "EQUADBTC with slash delimiter", symbol: "EQUADBTC", delimiter: "/", expectedBase: "EQUAD", expectedQuote: "BTC"},
		{name: "HTDFUSDT with slash delimiter", symbol: "HTDFUSDT", delimiter: "/", expectedBase: "HTDF", expectedQuote: "USDT"},
		{name: "BETHERETH with slash delimiter", symbol: "BETHERETH", delimiter: "/", expectedBase: "BETHER", expectedQuote: "ETH"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := MatchPairsWithNoDelimiter(tc.symbol, pairs, PairFormat{Uppercase: true, Delimiter: tc.delimiter})
			require.NoError(t, err, "MatchPairsWithNoDelimiter must not error")
			assert.Equal(t, tc.expectedBase, p.Base.String(), "MatchPairsWithNoDelimiter should return the expected base")
			assert.Equal(t, tc.expectedQuote, p.Quote.String(), "MatchPairsWithNoDelimiter should return the expected quote")
		})
	}
}

func TestPairFormat_Format(t *testing.T) {
	type fields struct {
		Uppercase bool
		Delimiter string
		Separator string
	}
	tests := []struct {
		name   string
		fields fields
		arg    Pair
		want   string
	}{
		{
			name:   "empty",
			fields: fields{},
			arg:    EMPTYPAIR,
			want:   "",
		},
		{
			name:   "empty format",
			fields: fields{},
			arg: Pair{
				Delimiter: "<>",
				Base:      AAA,
				Quote:     BTC,
			},
			want: "aaabtc",
		},
		{
			name: "format",
			fields: fields{
				Uppercase: true,
				Delimiter: "!!!",
			},
			arg: Pair{
				Delimiter: "<>",
				Base:      AAA,
				Quote:     BTC,
			},
			want: "AAA!!!BTC",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &PairFormat{
				Uppercase: tt.fields.Uppercase,
				Delimiter: tt.fields.Delimiter,
				Separator: tt.fields.Separator,
			}
			if got := f.Format(tt.arg); got != tt.want {
				t.Errorf("PairFormat.Format() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOther(t *testing.T) {
	received, err := NewPair(DAI, XRP).Other(DAI)
	if err != nil {
		t.Fatal(err)
	}
	if !received.Equal(XRP) {
		t.Fatal("unexpected value")
	}
	received, err = NewPair(DAI, XRP).Other(XRP)
	if err != nil {
		t.Fatal(err)
	}
	if !received.Equal(DAI) {
		t.Fatal("unexpected value")
	}
	_, err = NewPair(DAI, XRP).Other(BTC)
	require.ErrorIs(t, err, ErrCurrencyCodeEmpty)
}

func TestIsPopulated(t *testing.T) {
	if receiver := NewBTCUSDT().IsPopulated(); !receiver {
		t.Fatal("unexpected value")
	}
	if receiver := NewPair(BTC, NewCode("USD-1245")).IsPopulated(); !receiver {
		t.Fatal("unexpected value")
	}
	if receiver := NewPair(BTC, EMPTYCODE).IsPopulated(); receiver {
		t.Fatal("unexpected value")
	}
	if receiver := NewPair(EMPTYCODE, EMPTYCODE).IsPopulated(); receiver {
		t.Fatal("unexpected value")
	}
}

func TestGetOrderParameters(t *testing.T) {
	t.Parallel()

	p := NewBTCUSDT()
	testCases := []struct {
		name           string
		Pair           Pair
		currency       Code
		market         bool
		selling        bool
		expectedParams *OrderParameters
		expectedError  error
	}{
		{name: "empty pair", expectedError: ErrCurrencyPairEmpty},
		{name: "empty currency", Pair: p, expectedError: ErrCurrencyCodeEmpty},
		{name: "currency not in pair", Pair: p, currency: XRP, selling: true, market: true, expectedError: ErrCurrencyNotAssociatedWithPair},

		{name: "market sell BTC", Pair: p, currency: BTC, selling: true, market: true, expectedParams: &OrderParameters{SellingCurrency: BTC, PurchasingCurrency: USDT, IsBuySide: false, IsAskLiquidity: false, Pair: p}},
		{name: "market buy BTC", Pair: p, currency: BTC, selling: false, market: true, expectedParams: &OrderParameters{SellingCurrency: USDT, PurchasingCurrency: BTC, IsBuySide: true, IsAskLiquidity: true, Pair: p}},
		{name: "limit sell BTC", Pair: p, currency: BTC, selling: true, market: false, expectedParams: &OrderParameters{SellingCurrency: BTC, PurchasingCurrency: USDT, IsBuySide: false, IsAskLiquidity: true, Pair: p}},
		{name: "limit buy BTC", Pair: p, currency: BTC, selling: false, market: false, expectedParams: &OrderParameters{SellingCurrency: USDT, PurchasingCurrency: BTC, IsBuySide: true, IsAskLiquidity: false, Pair: p}},

		{name: "market sell USDT", Pair: p, currency: USDT, selling: true, market: true, expectedParams: &OrderParameters{SellingCurrency: USDT, PurchasingCurrency: BTC, IsBuySide: true, IsAskLiquidity: true, Pair: p}},
		{name: "market buy USDT", Pair: p, currency: USDT, selling: false, market: true, expectedParams: &OrderParameters{SellingCurrency: BTC, PurchasingCurrency: USDT, IsBuySide: false, IsAskLiquidity: false, Pair: p}},
		{name: "limit sell USDT", Pair: p, currency: USDT, selling: true, market: false, expectedParams: &OrderParameters{SellingCurrency: USDT, PurchasingCurrency: BTC, IsBuySide: true, IsAskLiquidity: false, Pair: p}},
		{name: "limit buy USDT", Pair: p, currency: USDT, selling: false, market: false, expectedParams: &OrderParameters{SellingCurrency: BTC, PurchasingCurrency: USDT, IsBuySide: false, IsAskLiquidity: true, Pair: p}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var resp *OrderParameters
			var err error
			switch {
			case tc.market && tc.selling:
				resp, err = tc.Pair.MarketSellOrderParameters(tc.currency)
			case tc.market && !tc.selling:
				resp, err = tc.Pair.MarketBuyOrderParameters(tc.currency)
			case !tc.market && tc.selling:
				resp, err = tc.Pair.LimitSellOrderParameters(tc.currency)
			case !tc.market && !tc.selling:
				resp, err = tc.Pair.LimitBuyOrderParameters(tc.currency)
			}
			require.ErrorIs(t, err, tc.expectedError)

			if tc.expectedParams == nil {
				if resp != nil {
					t.Fatalf("received %v, expected nil", resp)
				}
				return
			}

			if resp.SellingCurrency != tc.expectedParams.SellingCurrency {
				t.Fatalf("SellingCurrency received %v, expected %v", resp.SellingCurrency, tc.expectedParams.SellingCurrency)
			}

			if resp.PurchasingCurrency != tc.expectedParams.PurchasingCurrency {
				t.Fatalf("PurchasingCurrency received %v, expected %v", resp.PurchasingCurrency, tc.expectedParams.PurchasingCurrency)
			}

			if resp.IsBuySide != tc.expectedParams.IsBuySide {
				t.Fatalf("BuySide received %v, expected %v", resp.IsBuySide, tc.expectedParams.IsBuySide)
			}

			if resp.IsAskLiquidity != tc.expectedParams.IsAskLiquidity {
				t.Fatalf("AskLiquidity received %v, expected %v", resp.IsAskLiquidity, tc.expectedParams.IsAskLiquidity)
			}

			if resp.Pair != tc.expectedParams.Pair {
				t.Fatalf("Pair received %v, expected %v", resp.Pair, tc.expectedParams.Pair)
			}
		})
	}
}

func TestIsAssociated(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		Pair           Pair
		associate      Pair
		expectedResult bool
	}{
		{name: "same pair", Pair: NewBTCUSDT(), associate: NewBTCUSDT(), expectedResult: true},
		{name: "reversed pair", Pair: NewPair(USDT, BTC), associate: NewBTCUSDT(), expectedResult: true},
		{name: "reversed associate", Pair: NewBTCUSDT(), associate: NewPair(USDT, BTC), expectedResult: true},
		{name: "shared quote", Pair: NewBTCUSDT(), associate: NewPair(XRP, USDT), expectedResult: true},
		{name: "no shared currency", Pair: NewPair(BTC, LTC), associate: NewPair(XRP, USDT), expectedResult: false},
		{name: "quote is the associate's base", Pair: NewPair(MA, LTC), associate: NewPair(LTC, USDT), expectedResult: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.expectedResult, tc.Pair.IsAssociated(tc.associate), "IsAssociated should return the expected result")
		})
	}
}

func TestPair_GetFormatting(t *testing.T) {
	t.Parallel()
	pFmt, err := NewBTCUSDT().GetFormatting()
	require.NoError(t, err)
	assert.True(t, pFmt.Uppercase)
	assert.Empty(t, pFmt.Delimiter)

	pFmt, err = NewPairWithDelimiter("eth", "usdt", "/").GetFormatting()
	require.NoError(t, err)
	assert.False(t, pFmt.Uppercase)
	assert.Equal(t, "/", pFmt.Delimiter)

	_, err = NewPairWithDelimiter("eth", "USDT", "/").GetFormatting()
	require.ErrorIs(t, err, errPairFormattingInconsistent)

	pFmt, err = EMPTYPAIR.GetFormatting()
	require.NoError(t, err)
	assert.Equal(t, EMPTYFORMAT, pFmt)

	pFmt, err = NewPairWithDelimiter("eth", "420", "/").GetFormatting()
	require.NoError(t, err)
	assert.False(t, pFmt.Uppercase)

	pFmt, err = NewPairWithDelimiter("ETH", "420", "/").GetFormatting()
	require.NoError(t, err)
	assert.True(t, pFmt.Uppercase)

	pFmt, err = NewPairWithDelimiter("420", "eth", "/").GetFormatting()
	require.NoError(t, err)
	assert.False(t, pFmt.Uppercase)

	pFmt, err = NewPairWithDelimiter("420", "ETH", "/").GetFormatting()
	require.NoError(t, err)
	assert.True(t, pFmt.Uppercase)
}

func TestNewBTCUSD(t *testing.T) {
	t.Parallel()
	p := NewBTCUSD()
	if !p.Base.Equal(BTC) {
		t.Fatal("expected base BTC from function NewBTCUSD")
	}
	if !p.Quote.Equal(USD) {
		t.Fatal("expected quote USD from function NewBTCUSD")
	}
}

func TestNewBTCUSDT(t *testing.T) {
	t.Parallel()
	p := NewBTCUSDT()
	if !p.Base.Equal(BTC) {
		t.Fatal("expected base BTC from function NewBTCUSDT")
	}
	if !p.Quote.Equal(USDT) {
		t.Fatal("expected quote USDT from function NewBTCUSDT")
	}
}
