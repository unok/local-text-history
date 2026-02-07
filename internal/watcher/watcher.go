package watcher

import (
	"bytes"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/fsnotify/fsnotify"
)

// SnapshotSaver is called when a file change should be persisted.
type SnapshotSaver func(filePath string, content []byte) (bool, error)

// RenameSaver is called when a file rename is detected.
type RenameSaver func(oldPath, newPath string) (string, error)

// Config holds watcher configuration.
type Config struct {
	WatchDirs       []string
	Extensions      []string
	ExcludePatterns []string
	DebounceSec     int
	MaxFileSize     int64
}

// pendingRename tracks a Rename event waiting for a matching Create.
type pendingRename struct {
	oldPath   string
	timestamp time.Time
}

// Watcher monitors directories for file changes and triggers snapshots.
type Watcher struct {
	fsWatcher      *fsnotify.Watcher
	config         Config
	save           SnapshotSaver
	saveRename     RenameSaver
	timers         map[string]*time.Timer
	mu             sync.Mutex
	extSet         map[string]struct{}
	OnSnapshot     func(filePath string)
	OnRename       func(oldPath, newPath string)
	pendingRenames map[string]pendingRename
}

// New creates a Watcher with the given configuration and save function.
func New(cfg Config, save SnapshotSaver) (*Watcher, error) {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("creating fsnotify watcher: %w", err)
	}

	extSet := make(map[string]struct{}, len(cfg.Extensions))
	for _, ext := range cfg.Extensions {
		extSet[ext] = struct{}{}
	}

	w := &Watcher{
		fsWatcher:      fsw,
		config:         cfg,
		save:           save,
		timers:         make(map[string]*time.Timer),
		extSet:         extSet,
		pendingRenames: make(map[string]pendingRename),
	}

	for _, dir := range cfg.WatchDirs {
		if err := w.addDirRecursive(dir); err != nil {
			fsw.Close()
			return nil, fmt.Errorf("adding watch directory %q: %w", dir, err)
		}
	}

	return w, nil
}

// SetRenameSaver sets the function to call when a rename is detected.
func (w *Watcher) SetRenameSaver(saver RenameSaver) {
	w.saveRename = saver
}

// Run starts the event loop. It blocks until the done channel is closed.
func (w *Watcher) Run(done <-chan struct{}) {
	for {
		select {
		case <-done:
			return
		case event, ok := <-w.fsWatcher.Events:
			if !ok {
				return
			}
			w.handleEvent(event)
		case err, ok := <-w.fsWatcher.Errors:
			if !ok {
				return
			}
			log.Printf("watcher error: %v", err)
		}
	}
}

// Close stops the watcher and cancels all pending timers.
func (w *Watcher) Close() error {
	w.mu.Lock()
	for _, timer := range w.timers {
		timer.Stop()
	}
	w.timers = nil
	w.pendingRenames = nil
	w.mu.Unlock()
	return w.fsWatcher.Close()
}

// renameTimeout is how long to wait for a Create event after a Rename event.
const renameTimeout = 500 * time.Millisecond

func (w *Watcher) handleEvent(event fsnotify.Event) {
	// Handle Rename events: track pending renames
	if event.Has(fsnotify.Rename) {
		w.mu.Lock()
		w.pendingRenames[event.Name] = pendingRename{
			oldPath:   event.Name,
			timestamp: time.Now(),
		}
		w.mu.Unlock()

		// Schedule cleanup of stale pending renames
		time.AfterFunc(renameTimeout, func() {
			w.mu.Lock()
			if pr, ok := w.pendingRenames[event.Name]; ok {
				if time.Since(pr.timestamp) >= renameTimeout {
					delete(w.pendingRenames, event.Name)
				}
			}
			w.mu.Unlock()
		})
		return
	}

	// Handle new directory creation: add it to the watch list
	if event.Has(fsnotify.Create) {
		info, err := os.Stat(event.Name)
		if err == nil && info.IsDir() {
			if !w.isExcluded(event.Name) {
				if err := w.addDirRecursive(event.Name); err != nil {
					log.Printf("failed to watch new directory %s: %v", event.Name, err)
				}
			}
			return
		}

		// Check if this Create follows a Rename (file was moved)
		if w.tryMatchRename(event.Name) {
			// Rename matched and processed; still take a snapshot of the new file
			w.scheduleSnapshotIfTrackable(event.Name)
			return
		}
	}

	// Only process Write and Create events for files
	if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) {
		return
	}

	if !w.shouldTrack(event.Name) {
		return
	}

	w.scheduleSnapshot(event.Name)
}

// tryMatchRename checks if a Create event at newPath matches any pending Rename.
// It pairs Rename+Create events by checking if the old path was a tracked file
// with the same extension in the same directory.
func (w *Watcher) tryMatchRename(newPath string) bool {
	if w.saveRename == nil {
		return false
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	for oldPath, pr := range w.pendingRenames {
		if time.Since(pr.timestamp) > renameTimeout {
			delete(w.pendingRenames, oldPath)
			continue
		}

		if w.matchesPendingRename(oldPath) {
			delete(w.pendingRenames, oldPath)

			// Save rename record (outside lock via goroutine to avoid deadlock)
			go w.processRename(oldPath, newPath)
			return true
		}
	}

	return false
}

// matchesPendingRename checks if the old path was a tracked file,
// meaning a Rename event on it should be paired with the subsequent Create event.
func (w *Watcher) matchesPendingRename(oldPath string) bool {
	return w.shouldTrackPath(oldPath)
}

// shouldTrackPath checks if a path should be tracked (without checking if file exists).
func (w *Watcher) shouldTrackPath(filePath string) bool {
	if len(w.extSet) > 0 {
		ext := filepath.Ext(filePath)
		if _, ok := w.extSet[ext]; !ok {
			return false
		}
	}
	return !w.isExcluded(filePath)
}

// processRename saves the rename record and triggers callbacks.
func (w *Watcher) processRename(oldPath, newPath string) {
	_, err := w.saveRename(oldPath, newPath)
	if err != nil {
		log.Printf("failed to save rename %s -> %s: %v", oldPath, newPath, err)
		return
	}
	log.Printf("rename recorded: %s -> %s", oldPath, newPath)
	if w.OnRename != nil {
		w.OnRename(oldPath, newPath)
	}
}

// scheduleSnapshotIfTrackable schedules a snapshot only if the file should be tracked.
func (w *Watcher) scheduleSnapshotIfTrackable(filePath string) {
	if !w.shouldTrack(filePath) {
		return
	}
	w.scheduleSnapshot(filePath)
}

func (w *Watcher) scheduleSnapshot(filePath string) {
	debounce := time.Duration(w.config.DebounceSec) * time.Second

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.timers == nil {
		return
	}

	if timer, exists := w.timers[filePath]; exists {
		timer.Stop()
	}

	w.timers[filePath] = time.AfterFunc(debounce, func() {
		w.takeSnapshot(filePath)
		w.mu.Lock()
		delete(w.timers, filePath)
		w.mu.Unlock()
	})
}

func (w *Watcher) takeSnapshot(filePath string) {
	info, err := os.Stat(filePath)
	if err != nil {
		// File may have been deleted between event and snapshot
		return
	}

	if info.Size() > w.config.MaxFileSize {
		return
	}

	if info.Size() == 0 {
		return
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		log.Printf("failed to read file %s: %v", filePath, err)
		return
	}

	if isBinary(content) {
		return
	}

	saved, err := w.save(filePath, content)
	if err != nil {
		log.Printf("failed to save snapshot for %s: %v", filePath, err)
		return
	}
	if saved {
		log.Printf("snapshot saved: %s", filePath)
		if w.OnSnapshot != nil {
			go w.OnSnapshot(filePath)
		}
	}
}

func (w *Watcher) addDirRecursive(root string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if w.isExcluded(path) {
			return fs.SkipDir
		}
		return w.fsWatcher.Add(path)
	})
}

// shouldTrack returns true if the file should be tracked based on
// extension and exclude pattern filters.
func (w *Watcher) shouldTrack(filePath string) bool {
	// When extensions are configured, check the file extension
	if len(w.extSet) > 0 {
		ext := filepath.Ext(filePath)
		if _, ok := w.extSet[ext]; !ok {
			return false
		}
	}
	return !w.isExcluded(filePath)
}

// isExcluded returns true if the path matches any exclude pattern.
func (w *Watcher) isExcluded(filePath string) bool {
	for _, pattern := range w.config.ExcludePatterns {
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
	return bytes.ContainsRune(data[:checkLen], 0)
}
