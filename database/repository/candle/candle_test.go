package candle

import (
	"errors"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/database"
	"github.com/thrasher-corp/gocryptotrader/database/drivers"
	"github.com/thrasher-corp/gocryptotrader/database/repository/exchange"
	"github.com/thrasher-corp/gocryptotrader/database/testhelpers"
)

var (
	verbose       = false
	testExchanges = []exchange.Details{
		{
			Name: "one",
		},
		{
			Name: "two",
		},
	}
)

func TestMain(m *testing.M) {
	if verbose {
		err := testhelpers.EnableVerboseTestOutput()
		if err != nil {
			fmt.Printf("failed to enable verbose test output: %v", err)
			os.Exit(1)
		}
	}

	var err error
	testhelpers.PostgresTestDatabase = testhelpers.GetConnectionDetails()
	testhelpers.TempDir, err = os.MkdirTemp("", "gct-temp")
	if err != nil {
		fmt.Printf("failed to create temp file: %v", err)
		os.Exit(1)
	}

	t := m.Run()

	err = os.RemoveAll(testhelpers.TempDir)
	if err != nil {
		fmt.Printf("Failed to remove temp db file: %v", err)
	}

	os.Exit(t)
}

func TestInsert(t *testing.T) {
	testCases := []struct {
		name   string
		config *database.Config
		seedDB func(includeOHLCVData bool) error
		runner func(t *testing.T)
		closer func(dbConn *database.Instance) error
	}{
		{
			name:   "postgresql",
			config: testhelpers.PostgresTestDatabase,
			seedDB: seedDB,
		},
		{
			name: "SQLite",
			config: &database.Config{
				Driver:            database.DBSQLite3,
				ConnectionDetails: drivers.ConnectionDetails{Database: "./testdb"},
			},
			seedDB: seedDB,
		},
	}

	for x := range testCases {
		test := testCases[x]

		t.Run(test.name, func(t *testing.T) {
			if !testhelpers.CheckValidConfig(&test.config.ConnectionDetails) {
				t.Skip("database not configured skipping test")
			}

			dbConn, err := testhelpers.ConnectToDatabase(test.config)
			require.NoError(t, err)

			if test.seedDB != nil {
				err = test.seedDB(false)
				require.NoError(t, err)
			}

			data, err := genOHCLVData()
			require.NoError(t, err)

			r, err := Insert(&data)
			require.NoError(t, err)

			if r != 365 {
				t.Errorf("unexpected number inserted: %v", r)
			}

			d, err := DeleteCandles(&data)
			require.NoError(t, err)
			if d != 365 {
				t.Errorf("unexpected number deleted: %v", d)
			}

			err = testhelpers.CloseDatabase(dbConn)
			assert.NoError(t, err)
		})
	}
}

func TestInsertFromCSV(t *testing.T) {
	testCases := []struct {
		name   string
		config *database.Config
		seedDB func(includeOHLCVData bool) error
		runner func(t *testing.T)
		closer func(dbConn *database.Instance) error
	}{
		{
			name:   "postgresql",
			config: testhelpers.PostgresTestDatabase,
			seedDB: seedDB,
		},
		{
			name: "SQLite",
			config: &database.Config{
				Driver:            database.DBSQLite3,
				ConnectionDetails: drivers.ConnectionDetails{Database: "./testdb"},
			},
			seedDB: seedDB,
		},
	}

	for x := range testCases {
		test := testCases[x]

		t.Run(test.name, func(t *testing.T) {
			if !testhelpers.CheckValidConfig(&test.config.ConnectionDetails) {
				t.Skip("database not configured skipping test")
			}

			dbConn, err := testhelpers.ConnectToDatabase(test.config)
			require.NoError(t, err)

			if test.seedDB != nil {
				err = test.seedDB(false)
				require.NoError(t, err)
			}

			exchange.ResetExchangeCache()
			testFile := filepath.Join("..", "..", "..", "testdata", "binance_BTCUSDT_24h_2019_01_01_2020_01_01.csv")
			count, err := InsertFromCSV(testExchanges[0].Name, "BTC", "USDT", 86400, "spot", testFile)
			require.NoError(t, err)
			if count != 365 {
				t.Fatalf("expected 365 results to be inserted received: %v", count)
			}

			err = testhelpers.CloseDatabase(dbConn)
			assert.NoError(t, err)
		})
	}
}

func TestSeries(t *testing.T) {
	testCases := []struct {
		name   string
		config *database.Config
		seedDB func(includeOHLCVData bool) error
		runner func(t *testing.T)
		closer func(dbConn *database.Instance) error
	}{
		{
			name:   "postgresql",
			config: testhelpers.PostgresTestDatabase,
			seedDB: seedDB,
		},
		{
			name: "SQLite",
			config: &database.Config{
				Driver:            database.DBSQLite3,
				ConnectionDetails: drivers.ConnectionDetails{Database: "./testdb"},
			},
			seedDB: seedDB,
		},
	}

	for x := range testCases {
		test := testCases[x]

		t.Run(test.name, func(t *testing.T) {
			if !testhelpers.CheckValidConfig(&test.config.ConnectionDetails) {
				t.Skip("database not configured skipping test")
			}

			dbConn, err := testhelpers.ConnectToDatabase(test.config)
			require.NoError(t, err)

			if test.seedDB != nil {
				err = test.seedDB(true)
				require.NoError(t, err)
			}

			start := time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC)
			end := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
			ret, err := Series(testExchanges[0].Name,
				"BTC", "USDT",
				86400, "spot",
				start, end)
			require.NoError(t, err)
			if len(ret.Candles) != 365 {
				t.Errorf("unexpected number of results received:  %v", len(ret.Candles))
			}

			_, err = Series("", "", "", 0, "", start, end)
			require.ErrorIs(t, err, errInvalidInput)

			_, err = Series(testExchanges[0].Name,
				"BTC", "MOON",
				864000, "spot",
				start, end)
			// This allows either errInvalidInput or ErrNoCandleDataFound, or no error if data magically appears for MOON.
			// If an error occurs, it must be one of the two.
			if err != nil {
				assert.True(t, errors.Is(err, errInvalidInput) || errors.Is(err, ErrNoCandleDataFound), "Unexpected error: %v", err)
			}
			assert.NoError(t, testhelpers.CloseDatabase(dbConn))
		})
	}
}

func seedDB(includeOHLCVData bool) error {
	err := exchange.InsertMany(testExchanges)
	if err != nil {
		return err
	}

	if includeOHLCVData {
		exchange.ResetExchangeCache()
		data, err := genOHCLVData()
		if err != nil {
			return err
		}
		_, err = Insert(&data)
		return err
	}
	return nil
}

func genOHCLVData() (out Item, err error) {
	exchangeUUID, err := exchange.UUIDByName(testExchanges[0].Name)
	if err != nil {
		return
	}
	out.ExchangeID = exchangeUUID.String()
	out.Base = currency.BTC.String()
	out.Quote = currency.USDT.String()
	out.Interval = 86400
	out.Asset = "spot"

	start := time.Date(2019, 1, 1, 0, 0, 0, 0, time.UTC)
	for x := range 365 {
		out.Candles = append(out.Candles, Candle{
			Timestamp:        start.Add(time.Hour * 24 * time.Duration(x)),
			Open:             1000,
			High:             1000,
			Low:              1000,
			Close:            1000,
			Volume:           1000,
			ValidationIssues: "hello world!",
		})
	}

	return out, nil
}
