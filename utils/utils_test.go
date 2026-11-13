package utils

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdjustGoMaxProcs(t *testing.T) {
	curr := runtime.GOMAXPROCS(-1)
	numCPUs := runtime.NumCPU()

	// Cases run in order: each changes the process-wide GOMAXPROCS
	for _, tc := range []struct {
		name     string
		setting  int
		expected int
	}{
		{name: "current setting", setting: curr, expected: curr},
		{name: "logical CPU count", setting: numCPUs, expected: numCPUs},
		{name: "excessive setting uses the CPU count", setting: 1000, expected: numCPUs},
		{name: "negative setting uses the CPU count", setting: -1, expected: numCPUs},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, AdjustGoMaxProcs(tc.setting), "AdjustGoMaxProcs must not error")
			assert.Equal(t, tc.expected, runtime.GOMAXPROCS(-1), "GOMAXPROCS should be set to the expected value")
		})
	}
}
