package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thrasher-corp/gocryptotrader/encoding/json"
)

func TestBooleanUnmarshal(t *testing.T) {
	t.Parallel()
	data := []byte(`{"value": true, "another_value": "true", "third_value": "false", "fourth_value": 1, "fifth_value": 0, "sixth_value": "1", "seventh_value": "0"}`)
	var result map[string]Boolean
	err := json.Unmarshal(data, &result)
	require.NoError(t, err)
	assert.True(t, result["value"].Bool())
	assert.True(t, result["another_value"].Bool())
	assert.False(t, result["third_value"].Bool())
	assert.True(t, result["fourth_value"].Bool())
	assert.False(t, result["fifth_value"].Bool())
	assert.True(t, result["sixth_value"].Bool())
	assert.False(t, result["seventh_value"].Bool())

	data = []byte(`{"value": "3"}`)
	err = json.Unmarshal(data, &result)
	require.ErrorIs(t, err, errInvalidBooleanValue)
}

func TestBooleanQuotedCaseVariants(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		input string
		want  bool
	}{
		{`"true"`, true},
		{`"True"`, true},
		{`"TRUE"`, true},
		{`"false"`, false},
		{`"False"`, false},
		{`"FALSE"`, false},
	} {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			var value Boolean
			require.NoError(t, json.Unmarshal([]byte(tc.input), &value), "documented quoted boolean must decode")
			assert.Equal(t, tc.want, value.Bool(), "boolean should retain its meaning regardless of the documented spelling")
		})
	}
	for _, input := range []string{`"TrUe"`, `"yes"`, `"False "`, `null`, `2`} {
		var value Boolean
		assert.ErrorIs(t, json.Unmarshal([]byte(input), &value), errInvalidBooleanValue, "unsupported values should retain the boolean sentinel")
	}
}
