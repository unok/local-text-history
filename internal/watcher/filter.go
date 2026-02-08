package watcher

import (
	"bytes"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// shouldTrack returns true if the file should be tracked based on
// its WatchSet membership, extension, and exclude pattern filters.
func (w *Watcher) shouldTrack(filePath string) bool {
	ws := w.findWatchSet(filePath)
	if ws == nil {
		return false
	}
	if len(ws.extSet) > 0 {
		ext := filepath.Ext(filePath)
		if _, ok := ws.extSet[ext]; !ok {
			return false
		}
	}
	return !w.isExcludedBy(filePath, ws.excludePatterns)
}

// isExcluded checks if a path matches any exclude pattern of its owning WatchSet.
// Used for directory-level exclusion during recursive watch registration.
// Paths that do not belong to any WatchSet are considered excluded.
func (w *Watcher) isExcluded(dirPath string) bool {
	ws := w.findWatchSet(dirPath)
	if ws == nil {
		return true
	}
	return w.isExcludedBy(dirPath, ws.excludePatterns)
}

// isExcludedBy returns true if the path matches any of the given exclude patterns.
func (w *Watcher) isExcludedBy(filePath string, patterns []string) bool {
	for _, pattern := range patterns {
		matched, err := doublestar.PathMatch(pattern, filePath)
		if err != nil {
			continue
		}
		if matched {
			return true
		}
		// Also try matching against just the relative-like path components
		// by checking if any suffix of the path matches.
		parts := strings.Split(filePath, string(filepath.Separator))
		for i := range parts {
			sub := strings.Join(parts[i:], string(filepath.Separator))
			matched, err = doublestar.PathMatch(pattern, sub)
			if err != nil {
				continue
			}
			if matched {
				return true
			}
		}
	}
	return false
}

// binaryCheckSize is the number of bytes to inspect for NUL bytes.
const binaryCheckSize = 8192

// isBinary returns true if the data contains a NUL byte (0x00) in
// the first 8KB, indicating a binary file (same heuristic as Git).
func isBinary(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	checkLen := len(data)
	if checkLen > binaryCheckSize {
		checkLen = binaryCheckSize
	}
	return bytes.IndexByte(data[:checkLen], 0) >= 0
}
