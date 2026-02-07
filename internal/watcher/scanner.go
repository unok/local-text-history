package watcher

import (
	"io/fs"
	"log"
	"path/filepath"
)

// tryStartScan attempts to register root for scanning. Returns true if scanning
// can proceed, false if already scanning or watcher is closed.
func (w *Watcher) tryStartScan(root string) bool {
	w.scanMu.Lock()
	defer w.scanMu.Unlock()
	if w.scanningDirs == nil {
		return false
	}
	if _, scanning := w.scanningDirs[root]; scanning {
		return false
	}
	w.scanningDirs[root] = struct{}{}
	return true
}

// finishScan removes root from the scanning set.
func (w *Watcher) finishScan(root string) {
	w.scanMu.Lock()
	defer w.scanMu.Unlock()
	if w.scanningDirs != nil {
		delete(w.scanningDirs, root)
	}
}

// scanExistingFiles walks a directory tree and takes snapshots of all trackable files.
// It is designed to be called asynchronously after a new directory is detected,
// to pick up files that may have been missed by fsnotify event-driven model.
func (w *Watcher) scanExistingFiles(root string) {
	if !w.tryStartScan(root) {
		return
	}
	defer w.finishScan(root)

	var scannedCount int
	if err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			log.Printf("scan: skipping %s: %v", path, err)
			if d != nil && d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		select {
		case <-w.closeCh:
			return fs.SkipAll
		default:
		}

		if d.IsDir() {
			if w.isExcluded(path) {
				return fs.SkipDir
			}
			return nil
		}

		if w.shouldTrack(path) {
			w.takeSnapshot(path)
			scannedCount++
		}
		return nil
	}); err != nil {
		log.Printf("scan walk error for %s: %v", root, err)
	}

	if scannedCount > 0 {
		log.Printf("scan completed: %s (%d files scanned)", root, scannedCount)
	}
}
