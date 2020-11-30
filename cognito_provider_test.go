package replica

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCredsStartExpired(t *testing.T) {
	assert.True(t, new(cognitoProvider).IsExpired())
}
