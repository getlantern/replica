package replica

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExpired(t *testing.T) {
	assert.True(t, creds.IsExpired())
}
