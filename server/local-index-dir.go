package server

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/anacrolix/generics"
)

// Handles local index database file management. Ended up separate to make unit testing trivial.
type localIndexDir struct {
	dir string
}

var (
	localIndexDbFileRegexp = regexp.MustCompile(regexp.QuoteMeta(localIndexFilenamePrefix) + "-.*sqlite$")
	// Matches database files and their temporary files.
	localIndexTempFileRegexp = regexp.MustCompile(regexp.QuoteMeta(localIndexFilenamePrefix) + "-.*sqlite")
)

// Returns the base file name of the latest index file.
func (me localIndexDir) getLatestIndex() (_ generics.Option[string], err error) {
	allIndexPaths, err := me.listAllIndexes()
	if err != nil {
		return
	}
	sort.Strings(allIndexPaths)
	if len(allIndexPaths) > 0 {
		return generics.Some(allIndexPaths[len(allIndexPaths)-1]), nil
	}
	return
}

func (me localIndexDir) listAllIndexes() (fileNames []string, err error) {
	entries, err := os.ReadDir(me.dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		matched := localIndexDbFileRegexp.MatchString(entry.Name())
		if !matched {
			continue
		}
		fileNames = append(fileNames, entry.Name())
	}
	return
}

func (me localIndexDir) deleteUnusedIndexFiles(currentIndexName generics.Option[string]) error {
	entries, err := os.ReadDir(me.dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		entryName := entry.Name()
		matched := localIndexTempFileRegexp.MatchString(entryName)
		if !matched {
			continue
		}
		if currentIndexName.Ok && strings.HasPrefix(entryName, currentIndexName.Value) {
			// This file is related to the current index, so don't delete it.
			continue
		}
		os.Remove(filepath.Join(me.dir, entryName))
	}
	return nil
}
