package fill

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSetup tests the setup function of the Fills struct
func TestSetup(t *testing.T) {
	fill := &Fills{}
	channel := make(chan any)
	fill.Setup(true, channel)

	assert.NotNil(t, fill.dataHandler, "fill.dataHandler should be set")
	assert.True(t, fill.fillsFeedEnabled, "fill.fillsFeedEnabled should be true after setup")
}

// TestUpdateDisabledFeed tests the Update function when fillsFeedEnabled is false
func TestUpdateDisabledFeed(t *testing.T) {
	channel := make(chan any, 1)
	fill := Fills{dataHandler: channel, fillsFeedEnabled: false}

	// Send a test data to the Update function
	testData := Data{Timestamp: time.Now(), Price: 15.2, Amount: 3.2}
	assert.ErrorIs(t, fill.Update(testData), ErrFeedDisabled)

	select {
	case <-channel:
		assert.Fail(t, "channel should remain empty when feed disabled")
	default:
	}
}

// TestUpdate tests the Update function of the Fills struct.
func TestUpdate(t *testing.T) {
	channel := make(chan any, 1)
	fill := &Fills{dataHandler: channel, fillsFeedEnabled: true}
	receivedData := Data{Timestamp: time.Now(), Price: 15.2, Amount: 3.2}
	require.NoError(t, fill.Update(receivedData), "Update must not return error when feed enabled")

	select {
	case data := <-channel:
		dataSlice, ok := data.([]Data)
		require.True(t, ok, "Update must place []Data on channel")
		assert.Equal(t, []Data{receivedData}, dataSlice, "Update should send received data through channel")
	default:
		assert.Fail(t, "channel should receive data when feed enabled")
	}
}

// TestUpdateNoData tests the Update function with no Data objects
func TestUpdateNoData(t *testing.T) {
	channel := make(chan any, 1)
	fill := &Fills{dataHandler: channel, fillsFeedEnabled: true}
	require.NoError(t, fill.Update(), "Update must not return error when no data supplied")

	select {
	case <-channel:
		assert.Fail(t, "channel should remain empty when no data provided")
	default:
		// pass, nothing to do
	}
}

// TestUpdateMultipleData tests the Update function with multiple Data objects
func TestUpdateMultipleData(t *testing.T) {
	channel := make(chan any, 2)
	fill := &Fills{dataHandler: channel, fillsFeedEnabled: true}
	receivedData := Data{Timestamp: time.Now(), Price: 15.2, Amount: 3.2}
	receivedData2 := Data{Timestamp: time.Now(), Price: 18.2, Amount: 9.0}
	require.NoError(t, fill.Update(receivedData, receivedData2), "Update must not return error with multiple data points")

	select {
	case data := <-channel:
		dataSlice, ok := data.([]Data)
		require.True(t, ok, "Update must place []Data on channel")
		assert.Equal(t, []Data{receivedData, receivedData2}, dataSlice, "Update should send all provided data through channel")
	default:
		assert.Fail(t, "channel should receive data when feed enabled")
	}
}
