package watcher

import (
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

const (
	saveRetryCount = 3
	saveRetryDelay = 1 * time.Second
	saveQueueSize  = 10000
)

// SnapshotSaver is called when a file change should be persisted.
type SnapshotSaver func(filePath string, content []byte) (bool, error)

// SnapshotBatchSaver saves multiple snapshots in a single transaction.
// Returns a saved flag and error for each input item.
type SnapshotBatchSaver func(filePaths []string, contents [][]byte) ([]bool, []error)

// RenameSaver is called when a file rename is detected.
type RenameSaver func(oldPath, newPath string) (string, error)

// saveJob represents a queued DB write operation.
type saveJob struct {
	filePath string
	content  []byte
	oldPath  string // rename only
	newPath  string // rename only
	rename   bool
}

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
	saveBatch      SnapshotBatchSaver
	saveRename     RenameSaver
	timers         map[string]*time.Timer
	mu             sync.Mutex
	extSet         map[string]struct{}
	OnSnapshot     func(filePath string)
	OnRename       func(oldPath, newPath string)
	pendingRenames map[string]pendingRename
	saveCh         chan saveJob
	closeCh        chan struct{}
	scanningDirs   map[string]struct{}
	scanMu         sync.Mutex
	scanWg         sync.WaitGroup
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
		saveCh:         make(chan saveJob, saveQueueSize),
		closeCh:        make(chan struct{}),
		scanningDirs:   make(map[string]struct{}),
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

// SetBatchSaver sets the function for bulk snapshot saving.
func (w *Watcher) SetBatchSaver(saver SnapshotBatchSaver) {
	w.saveBatch = saver
}

// Run starts the event loop. It blocks until the done channel is closed.
func (w *Watcher) Run(done <-chan struct{}) {
	go w.saveWorker(done)
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

// saveWorker processes DB write jobs, batching snapshots for bulk insert.
func (w *Watcher) saveWorker(done <-chan struct{}) {
	for {
		select {
		case <-done:
			w.processBatch(w.drainAll())
			return
		case job := <-w.saveCh:
			w.processBatch(w.drainBatch(job))
		}
	}
}

// drainBatch collects the first job plus any additional queued jobs without blocking.
func (w *Watcher) drainBatch(first saveJob) []saveJob {
	batch := []saveJob{first}
	for len(batch) < saveQueueSize {
		select {
		case j := <-w.saveCh:
			batch = append(batch, j)
		default:
			return batch
		}
	}
	return batch
}

// drainAll collects all remaining queued jobs without blocking.
func (w *Watcher) drainAll() []saveJob {
	var batch []saveJob
	for {
		select {
		case j := <-w.saveCh:
			batch = append(batch, j)
		default:
			return batch
		}
	}
}

// processBatch handles a batch of save jobs, using bulk insert for snapshots.
func (w *Watcher) processBatch(batch []saveJob) {
	if len(batch) == 0 {
		return
	}

	var snapshots []saveJob
	var renames []saveJob
	for _, j := range batch {
		if j.rename {
			renames = append(renames, j)
		} else {
			snapshots = append(snapshots, j)
		}
	}

	if len(snapshots) > 0 {
		w.processSnapshotBatch(snapshots)
	}
	for _, r := range renames {
		w.processSingleRename(r.oldPath, r.newPath)
	}
}

// processSnapshotBatch saves snapshots using bulk insert with retry fallback.
func (w *Watcher) processSnapshotBatch(snapshots []saveJob) {
	filePaths := make([]string, len(snapshots))
	contents := make([][]byte, len(snapshots))
	for i, s := range snapshots {
		filePaths[i] = s.filePath
		contents[i] = s.content
	}

	var savedSlice []bool
	var errSlice []error

	saver := w.saveBatch
	if saver == nil {
		// Fallback: save individually with retry
		savedSlice = make([]bool, len(snapshots))
		errSlice = make([]error, len(snapshots))
		for i := range snapshots {
			for attempt := range saveRetryCount {
				savedSlice[i], errSlice[i] = w.save(filePaths[i], contents[i])
				if errSlice[i] == nil {
					break
				}
				if !strings.Contains(errSlice[i].Error(), "database is locked") {
					break
				}
				if attempt < saveRetryCount-1 {
					time.Sleep(saveRetryDelay)
				}
			}
		}
	} else {
		for attempt := range saveRetryCount {
			savedSlice, errSlice = saver(filePaths, contents)
			if !w.hasDatabaseLockedError(errSlice) {
				break
			}
			if attempt < saveRetryCount-1 {
				time.Sleep(saveRetryDelay)
			}
		}
	}

	for i, s := range snapshots {
		if errSlice[i] != nil {
			log.Printf("failed to save snapshot for %s: %v", s.filePath, errSlice[i])
			continue
		}
		if savedSlice[i] {
			log.Printf("snapshot saved: %s", s.filePath)
			if w.OnSnapshot != nil {
				go w.OnSnapshot(s.filePath)
			}
		}
	}
}

func (w *Watcher) hasDatabaseLockedError(errs []error) bool {
	for _, err := range errs {
		if err != nil && strings.Contains(err.Error(), "database is locked") {
			return true
		}
	}
	return false
}

// processSingleRename saves a single rename record with retry.
func (w *Watcher) processSingleRename(oldPath, newPath string) {
	var newFileID string
	var err error
	for attempt := range saveRetryCount {
		newFileID, err = w.saveRename(oldPath, newPath)
		if err == nil {
			break
		}
		if !strings.Contains(err.Error(), "database is locked") {
			break
		}
		if attempt < saveRetryCount-1 {
			time.Sleep(saveRetryDelay)
		}
	}
	if err != nil {
		log.Printf("failed to save rename %s -> %s: %v", oldPath, newPath, err)
		return
	}
	if newFileID == "" {
		// Old file not tracked (e.g. temp file renamed to real file) — skip silently
		return
	}
	log.Printf("rename recorded: %s -> %s", oldPath, newPath)
	if w.OnRename != nil {
		w.OnRename(oldPath, newPath)
	}
}

// Close stops the watcher and cancels all pending timers.
func (w *Watcher) Close() error {
	close(w.closeCh)
	w.scanWg.Wait()
	w.mu.Lock()
	for _, timer := range w.timers {
		timer.Stop()
	}
	w.timers = nil
	w.pendingRenames = nil
	w.mu.Unlock()
	w.scanMu.Lock()
	w.scanningDirs = nil
	w.scanMu.Unlock()
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
				w.scanWg.Add(1)
				go func() {
					defer w.scanWg.Done()
					w.scanExistingFiles(event.Name)
				}()
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
	return w.shouldTrack(oldPath)
}

// processRename queues a rename record for saving.
func (w *Watcher) processRename(oldPath, newPath string) {
	w.saveCh <- saveJob{rename: true, oldPath: oldPath, newPath: newPath}
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

	w.saveCh <- saveJob{filePath: filePath, content: content}
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

