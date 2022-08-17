package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/anacrolix/generics"
	qt "github.com/frankban/quicktest"
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

func TestLocalIndexDir(t *testing.T) {
	dir := t.TempDir()
	os.Create(filepath.Join(dir, "a"))
	os.Create(filepath.Join(dir, "replica-local-index-20220629-090105.sqlite-wal"))
	os.Create(filepath.Join(dir, "replica-local-index-13371337-94n9574.sqlite"))
	os.Create(filepath.Join(dir, "replica-local-index-20220817-124414.sqlite"))
	os.Create(filepath.Join(dir, "replica-local-index-20220817-124414.sqlite-shm"))
	liDir := localIndexDir{dir}
	indexes, err := liDir.listAllIndexes()
	c := qt.New(t)
	c.Assert(err, qt.IsNil)
	c.Assert(indexes, qt.HasLen, 2)
	entries, err := os.ReadDir(dir)
	c.Assert(entries, qt.HasLen, 5)
	latest, err := liDir.getLatestIndex()
	c.Check(err, qt.IsNil)
	c.Check(latest.Ok, qt.IsTrue)
	c.Check(latest, qt.Equals, generics.Some("replica-local-index-20220817-124414.sqlite"))
	err = liDir.deleteUnusedIndexFiles(latest)
	c.Assert(err, qt.IsNil)
	entries, err = os.ReadDir(dir)
	// Only the latest index files, and non-index files, should remain.
	c.Assert(entries, qt.HasLen, 3)
}
