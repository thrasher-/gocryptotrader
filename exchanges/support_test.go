package exchange

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsSupported(t *testing.T) {
	assert.True(t, IsSupported("BiTStaMp"), "IsSupported should return true for supported exchange")
	assert.False(t, IsSupported("meowexch"), "IsSupported should return false for unsupported exchange")
}
