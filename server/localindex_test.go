package server

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEscapeFts5String(t *testing.T) {
	for _, tc := range []struct {
		input          string
		expectedOutput string
	}{
		{
			input:          `bunnyfoofoo`,
			expectedOutput: `"bunnyfoofoo"`,
		},
		{
			input:          `bunny"foofoo`,
			expectedOutput: `"bunny" "foofoo"`,
		},
		{
			input:          `bunny"foo"foo`,
			expectedOutput: `"bunny" "foo" "foo"`,
		},
	} {
		require.Equal(t,
			tc.expectedOutput,
			escapeFts5QueryString(tc.input))
	}
}
