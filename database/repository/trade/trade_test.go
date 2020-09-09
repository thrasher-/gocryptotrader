package trade

import (
	"os"
	"testing"
	"time"

	"github.com/gofrs/uuid"

	"github.com/thrasher-corp/gocryptotrader/currency"
	"github.com/thrasher-corp/gocryptotrader/database"
	"github.com/thrasher-corp/gocryptotrader/database/drivers"
	"github.com/thrasher-corp/gocryptotrader/database/repository/exchange"
	"github.com/thrasher-corp/gocryptotrader/database/testhelpers"
	"github.com/thrasher-corp/gocryptotrader/exchanges/asset"
	"github.com/thrasher-corp/gocryptotrader/exchanges/order"
)

var (
	verbose       = true
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
		testhelpers.EnableVerboseTestOutput()
	}
	testhelpers.PostgresTestDatabase = testhelpers.GetConnectionDetails()

	t := m.Run()
	os.Exit(t)
}

func TestTrades(t *testing.T) {
	testCases := []struct {
		name   string
		config *database.Config
		seedDB func() error
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
			if err != nil {
				t.Fatal(err)
			}

			if test.seedDB != nil {
				err = test.seedDB()
				if err != nil {
					t.Error(err)
				}
			}

			tradeSqlTester(t)
			err = testhelpers.CloseDatabase(dbConn)
			if err != nil {
				t.Error(err)
			}
		})
	}
}

func tradeSqlTester(t *testing.T) {
	var trades []Data
	for i := 0; i < 20; i++ {
		uu, _ := uuid.NewV4()
		trades = append(trades, Data{
			ID:        uu.String(),
			Timestamp: time.Now().Unix(),
			Exchange:  testExchanges[0].Name,
			Base:      currency.BTC.String(),
			Quote:     currency.USD.String(),
			AssetType: asset.Spot.String(),
			Price:     float64(i * (i + 3)),
			Amount:    float64(i * (i + 2)),
			Side:      order.Buy.String(),
		})
	}
	err := Insert(trades...)
	if err != nil {
		t.Fatal(err)
	}
	if len(trades) == 0 {
		t.Fatal("somehow did not append trades")
	}
	_, err = GetByUUID(trades[0].ID)
	if err != nil {
		t.Error(err)
	}

	v, err := GetByExchangeInRange(testExchanges[0].Name, time.Now().Add(-time.Hour).Unix(), time.Now().Add(time.Hour).Unix())
	if err != nil {
		t.Error(err)
	}
	if len(v) == 0 {
		t.Error("Bad get!")
	}

	err = DeleteTrades(trades...)
	if err != nil {
		t.Error(err)
	}

	v, err = GetByExchangeInRange(testExchanges[0].Name, time.Now().Add(-time.Hour).Unix(), time.Now().Add(time.Hour).Unix())
	if err != nil {
		t.Error(err)
	}
	if len(v) != 0 {
		t.Errorf("should all be ded %v", v)
	}
}

func seedDB() error {
	err := exchange.InsertMany(testExchanges)
	if err != nil {
		return err
	}

	return nil
}