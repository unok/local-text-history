package watcher

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unok/local-text-history/internal/config"
)

// newTestConfig creates a single-WatchSet watcher Config for testing convenience.
func newTestConfig(dir string, extensions []string, excludePatterns []string, debounceSec int, maxFileSize int64) Config {
	return Config{
		WatchSets: []config.WatchSet{
			{
				Name:            "test",
				Dirs:            []string{dir},
				Extensions:      extensions,
				ExcludePatterns: excludePatterns,
				DebounceSec:     debounceSec,
				MaxFileSize:     maxFileSize,
			},
		},
	}
}

func TestShouldTrack_Extensions(t *testing.T) {
	dir := t.TempDir()
	cfg := newTestConfig(dir, []string{".go", ".ts"}, []string{}, 1, 1048576)
	w, err := New(cfg, func(path string, content []byte, maxSnapshots int) (bool, error) {
		return true, nil
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer w.Close()

	tests := []struct {
		path string
		want bool
	}{
		{filepath.Join(dir, "main.go"), true},
		{filepath.Join(dir, "app.ts"), true},
		{filepath.Join(dir, "readme.md"), false},
		{filepath.Join(dir, "image.png"), false},
		{filepath.Join(dir, "noext"), false},
	}

	for _, tt := range tests {
		got := w.shouldTrack(tt.path)
		if got != tt.want {
			t.Errorf("shouldTrack(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestShouldTrack_NoExtensions(t *testing.T) {
	dir := t.TempDir()
	cfg := newTestConfig(dir, nil, []string{"**/node_modules/**"}, 1, 1048576)
	w, err := New(cfg, func(path string, content []byte, maxSnapshots int) (bool, error) {
		return true, nil
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer w.Close()

	tests := []struct {
		path string
		want bool
	}{
		{filepath.Join(dir, "main.go"), true},
		{filepath.Join(dir, "app.ts"), true},
		{filepath.Join(dir, "readme.md"), true},
		{filepath.Join(dir, "image.png"), true},
		{filepath.Join(dir, "noext"), true},
		{filepath.Join(dir, "node_modules", "pkg", "index.js"), false},
	}

	for _, tt := range tests {
		got := w.shouldTrack(tt.path)
		if got != tt.want {
			t.Errorf("shouldTrack(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestShouldTrack_OutsideWatchSet(t *testing.T) {
	dir := t.TempDir()
	cfg := newTestConfig(dir, []string{".go"}, []string{}, 1, 1048576)
	w, err := New(cfg, func(path string, content []byte, maxSnapshots int) (bool, error) {
		return true, nil
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer w.Close()

	// A file outside all WatchSet dirs should not be tracked
	outsidePath := "/some/other/dir/main.go"
	if w.shouldTrack(outsidePath) {
		t.Errorf("shouldTrack(%q) = true, want false (outside WatchSet)", outsidePath)
	}
}

func TestIsExcluded(t *testing.T) {
	dir := t.TempDir()
	cfg := newTestConfig(dir, nil, []string{
		"**/node_modules/**",
		"**/.git/**",
		"**/*.min.js",
	}, 1, 1048576)
	w, err := New(cfg, func(path string, content []byte, maxSnapshots int) (bool, error) {
		return true, nil
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer w.Close()

	tests := []struct {
		path string
		want bool
	}{
		{filepath.Join(dir, "node_modules", "pkg", "index.js"), true},
		{filepath.Join(dir, ".git", "objects", "abc"), true},
		{filepath.Join(dir, "src", "app.min.js"), true},
		{filepath.Join(dir, "src", "main.go"), false},
		{filepath.Join(dir, "src", "app.js"), false},
	}

	for _, tt := range tests {
		got := w.isExcluded(tt.path)
		if got != tt.want {
			t.Errorf("isExcluded(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestIsExcluded_OutsideWatchSet(t *testing.T) {
	dir := t.TempDir()
	cfg := newTestConfig(dir, nil, []string{}, 1, 1048576)
	w, err := New(cfg, func(path string, content []byte, maxSnapshots int) (bool, error) {
		return true, nil
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer w.Close()

	// Paths outside any WatchSet should be considered excluded
	if !w.isExcluded("/some/other/dir") {
		t.Error("isExcluded for path outside WatchSet should return true")
	}
}

func TestIsBinary_TextFile(t *testing.T) {
	data := []byte("package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n")
	if isBinary(data) {
		t.Error("isBinary() = true for text data, want false")
	}
}

func TestIsBinary_BinaryFile(t *testing.T) {
	data := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D}
	if !isBinary(data) {
		t.Error("isBinary() = false for binary data with NUL, want true")
	}
}

func TestIsBinary_EmptyFile(t *testing.T) {
	if isBinary([]byte{}) {
		t.Error("isBinary() = true for empty data, want false")
	}
}

func TestIsBinary_NulAfter8KB(t *testing.T) {
	// NUL byte after the 8KB check window should not be detected
	data := make([]byte, 10000)
	for i := range data {
		data[i] = 'a'
	}
	data[9000] = 0x00
	if isBinary(data) {
		t.Error("isBinary() = true for NUL after 8KB, want false")
	}
}

func TestIsBinary_NulWithin8KB(t *testing.T) {
	data := make([]byte, 10000)
	for i := range data {
		data[i] = 'a'
	}
	data[4000] = 0x00
	if !isBinary(data) {
		t.Error("isBinary() = false for NUL within 8KB, want true")
	}
}

func TestWatcher_Debounce(t *testing.T) {
	dir := t.TempDir()

	var mu sync.Mutex
	var saved []string

	saver := func(path string, content []byte, maxSnapshots int) (bool, error) {
		mu.Lock()
		saved = append(saved, path)
		mu.Unlock()
		return true, nil
	}

	cfg := newTestConfig(dir, []string{".txt"}, []string{}, 1, 1048576)

	w, err := New(cfg, saver)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer w.Close()

	done := make(chan struct{})
	go w.Run(done)

	// Write to a file multiple times rapidly
	testFile := filepath.Join(dir, "test.txt")
	for i := range 5 {
		if err := os.WriteFile(testFile, []byte("content "+string(rune('0'+i))), 0o644); err != nil {
			t.Fatal(err)
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Wait for debounce to fire (1 second + buffer)
	time.Sleep(2 * time.Second)
	close(done)

	mu.Lock()
	defer mu.Unlock()

	// Debounce should collapse multiple writes into one snapshot
	if len(saved) != 1 {
		t.Errorf("debounce: got %d saves, want 1", len(saved))
	}
}

func TestWatcher_IgnoresLargeFiles(t *testing.T) {
	dir := t.TempDir()

	var mu sync.Mutex
	var saved []string

	saver := func(path string, content []byte, maxSnapshots int) (bool, error) {
		mu.Lock()
		saved = append(saved, path)
		mu.Unlock()
		return true, nil
	}

	cfg := newTestConfig(dir, []string{".txt"}, []string{}, 1, 100) // 100 bytes max

	w, err := New(cfg, saver)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer w.Close()

	done := make(chan struct{})
	go w.Run(done)

	// Write a large file
	largeContent := make([]byte, 200)
	testFile := filepath.Join(dir, "large.txt")
	if err := os.WriteFile(testFile, largeContent, 0o644); err != nil {
		t.Fatal(err)
	}

	time.Sleep(2 * time.Second)
	close(done)

	mu.Lock()
	defer mu.Unlock()

	if len(saved) != 0 {
		t.Errorf("large file: got %d saves, want 0", len(saved))
	}
}

func TestWatcher_ExcludedDirectory(t *testing.T) {
	dir := t.TempDir()
	nodeModules := filepath.Join(dir, "node_modules", "pkg")
	if err := os.MkdirAll(nodeModules, 0o755); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var saved []string

	saver := func(path string, content []byte, maxSnapshots int) (bool, error) {
		mu.Lock()
		saved = append(saved, path)
		mu.Unlock()
		return true, nil
	}

	cfg := newTestConfig(dir, []string{".js"}, []string{"**/node_modules/**"}, 1, 1048576)

	w, err := New(cfg, saver)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer w.Close()

	done := make(chan struct{})
	go w.Run(done)

	// Write to excluded directory
	testFile := filepath.Join(nodeModules, "index.js")
	if err := os.WriteFile(testFile, []byte("module.exports = {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	time.Sleep(2 * time.Second)
	close(done)

	mu.Lock()
	defer mu.Unlock()

	if len(saved) != 0 {
		t.Errorf("excluded dir: got %d saves, want 0", len(saved))
	}
}

func TestWatcher_SkipsEmptyFiles(t *testing.T) {
	dir := t.TempDir()

	var mu sync.Mutex
	var saved []string

	saver := func(path string, content []byte, maxSnapshots int) (bool, error) {
		mu.Lock()
		saved = append(saved, path)
		mu.Unlock()
		return true, nil
	}

	cfg := newTestConfig(dir, []string{".txt"}, []string{}, 1, 1048576)

	w, err := New(cfg, saver)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer w.Close()

	done := make(chan struct{})
	go w.Run(done)

	// Create an empty file
	testFile := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(testFile, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	time.Sleep(2 * time.Second)
	close(done)

	mu.Lock()
	defer mu.Unlock()

	if len(saved) != 0 {
		t.Errorf("empty file: got %d saves, want 0", len(saved))
	}
}

func TestWatcher_SavesAfterContentWritten(t *testing.T) {
	dir := t.TempDir()

	var mu sync.Mutex
	var saved []string

	saver := func(path string, content []byte, maxSnapshots int) (bool, error) {
		mu.Lock()
		saved = append(saved, path)
		mu.Unlock()
		return true, nil
	}

	cfg := newTestConfig(dir, []string{".txt"}, []string{}, 1, 1048576)

	w, err := New(cfg, saver)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer w.Close()

	done := make(chan struct{})
	go w.Run(done)

	// Create an empty file first
	testFile := filepath.Join(dir, "willwrite.txt")
	if err := os.WriteFile(testFile, []byte{}, 0o644); err != nil {
		t.Fatal(err)
	}

	// Wait for debounce — empty file should not be saved
	time.Sleep(2 * time.Second)

	mu.Lock()
	count := len(saved)
	mu.Unlock()

	if count != 0 {
		t.Fatalf("empty file phase: got %d saves, want 0", count)
	}

	// Write content to the file
	if err := os.WriteFile(testFile, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Wait for debounce — should be saved now
	time.Sleep(2 * time.Second)
	close(done)

	mu.Lock()
	defer mu.Unlock()

	if len(saved) != 1 {
		t.Errorf("after content written: got %d saves, want 1", len(saved))
	}
	if len(saved) == 1 && saved[0] != testFile {
		t.Errorf("saved file = %s, want %s", saved[0], testFile)
	}
}

func TestWatcher_IgnoresBinaryFiles(t *testing.T) {
	dir := t.TempDir()

	var mu sync.Mutex
	var saved []string

	saver := func(path string, content []byte, maxSnapshots int) (bool, error) {
		mu.Lock()
		saved = append(saved, path)
		mu.Unlock()
		return true, nil
	}

	cfg := newTestConfig(dir, nil, []string{}, 1, 1048576) // No extension filter

	w, err := New(cfg, saver)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer w.Close()

	done := make(chan struct{})
	go w.Run(done)

	// Write a text file — should be saved
	textFile := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(textFile, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Write a binary file — should be skipped
	binFile := filepath.Join(dir, "test.bin")
	binaryContent := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D}
	if err := os.WriteFile(binFile, binaryContent, 0o644); err != nil {
		t.Fatal(err)
	}

	time.Sleep(2 * time.Second)
	close(done)

	mu.Lock()
	defer mu.Unlock()

	if len(saved) != 1 {
		t.Errorf("binary detection: got %d saves, want 1 (only text file)", len(saved))
	}
	if len(saved) == 1 && saved[0] != textFile {
		t.Errorf("saved file = %s, want %s", saved[0], textFile)
	}
}

func TestWatcher_OnSnapshotCallback(t *testing.T) {
	dir := t.TempDir()

	var mu sync.Mutex
	var notified []string

	saver := func(path string, content []byte, maxSnapshots int) (bool, error) {
		return true, nil
	}

	cfg := newTestConfig(dir, []string{".txt"}, []string{}, 1, 1048576)

	w, err := New(cfg, saver)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer w.Close()

	w.OnSnapshot = func(filePath string) {
		mu.Lock()
		notified = append(notified, filePath)
		mu.Unlock()
	}

	done := make(chan struct{})
	go w.Run(done)

	testFile := filepath.Join(dir, "callback.txt")
	if err := os.WriteFile(testFile, []byte("trigger callback"), 0o644); err != nil {
		t.Fatal(err)
	}

	time.Sleep(2 * time.Second)
	close(done)

	// Wait briefly for the goroutine to complete
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(notified) != 1 {
		t.Errorf("OnSnapshot callback: got %d calls, want 1", len(notified))
	}
	if len(notified) == 1 && notified[0] != testFile {
		t.Errorf("notified file = %s, want %s", notified[0], testFile)
	}
}

func TestWatcher_OnSnapshotNotCalledOnDuplicate(t *testing.T) {
	dir := t.TempDir()

	var saveMu sync.Mutex
	var saveCount int

	saver := func(path string, content []byte, maxSnapshots int) (bool, error) {
		saveMu.Lock()
		saveCount++
		first := saveCount == 1
		saveMu.Unlock()
		// First call saves, second is a duplicate
		return first, nil
	}

	var mu sync.Mutex
	var notified []string

	cfg := newTestConfig(dir, []string{".txt"}, []string{}, 1, 1048576)

	w, err := New(cfg, saver)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer w.Close()

	w.OnSnapshot = func(filePath string) {
		mu.Lock()
		notified = append(notified, filePath)
		mu.Unlock()
	}

	done := make(chan struct{})
	go w.Run(done)

	testFile := filepath.Join(dir, "dup.txt")
	if err := os.WriteFile(testFile, []byte("first write"), 0o644); err != nil {
		t.Fatal(err)
	}

	time.Sleep(2 * time.Second)

	// Write same content again (saver returns false)
	if err := os.WriteFile(testFile, []byte("first write"), 0o644); err != nil {
		t.Fatal(err)
	}

	time.Sleep(2 * time.Second)
	close(done)

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// OnSnapshot should only be called once (the first save)
	if len(notified) != 1 {
		t.Errorf("OnSnapshot callback on duplicate: got %d calls, want 1", len(notified))
	}
}

func TestTakeSnapshot_RetriesOnDatabaseLocked(t *testing.T) {
	dir := t.TempDir()

	var attempts atomic.Int32

	saver := func(path string, content []byte, maxSnapshots int) (bool, error) {
		n := attempts.Add(1)
		if n < 3 {
			return false, errors.New("beginning transaction: database is locked")
		}
		return true, nil
	}

	cfg := newTestConfig(dir, []string{".txt"}, []string{}, 1, 1048576)

	w, err := New(cfg, saver)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer w.Close()

	var mu sync.Mutex
	var notified []string
	w.OnSnapshot = func(filePath string) {
		mu.Lock()
		notified = append(notified, filePath)
		mu.Unlock()
	}

	done := make(chan struct{})
	go w.Run(done)

	testFile := filepath.Join(dir, "retry.txt")
	if err := os.WriteFile(testFile, []byte("retry content"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Wait for debounce + retries (1s debounce + 2*1s retry delays + buffer)
	time.Sleep(5 * time.Second)
	close(done)

	time.Sleep(100 * time.Millisecond)

	if got := attempts.Load(); got != 3 {
		t.Errorf("save attempts = %d, want 3", got)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(notified) != 1 {
		t.Errorf("OnSnapshot callback: got %d calls, want 1", len(notified))
	}
}

func TestTakeSnapshot_NoRetryOnOtherErrors(t *testing.T) {
	dir := t.TempDir()

	var attempts atomic.Int32

	saver := func(path string, content []byte, maxSnapshots int) (bool, error) {
		attempts.Add(1)
		return false, errors.New("some other error")
	}

	cfg := newTestConfig(dir, []string{".txt"}, []string{}, 1, 1048576)

	w, err := New(cfg, saver)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer w.Close()

	done := make(chan struct{})
	go w.Run(done)

	testFile := filepath.Join(dir, "noretry.txt")
	if err := os.WriteFile(testFile, []byte("no retry content"), 0o644); err != nil {
		t.Fatal(err)
	}

	time.Sleep(3 * time.Second)
	close(done)

	if got := attempts.Load(); got != 1 {
		t.Errorf("save attempts = %d, want 1 (no retry for non-locked errors)", got)
	}
}

func TestTakeSnapshot_AllRetriesFail(t *testing.T) {
	dir := t.TempDir()

	var attempts atomic.Int32

	saver := func(path string, content []byte, maxSnapshots int) (bool, error) {
		attempts.Add(1)
		return false, errors.New("inserting file: database is locked")
	}

	cfg := newTestConfig(dir, []string{".txt"}, []string{}, 1, 1048576)

	w, err := New(cfg, saver)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer w.Close()

	var mu sync.Mutex
	var notified []string
	w.OnSnapshot = func(filePath string) {
		mu.Lock()
		notified = append(notified, filePath)
		mu.Unlock()
	}

	done := make(chan struct{})
	go w.Run(done)

	testFile := filepath.Join(dir, "allfail.txt")
	if err := os.WriteFile(testFile, []byte("fail content"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Wait for debounce + all retries (1s debounce + 2*1s retry delays + buffer)
	time.Sleep(5 * time.Second)
	close(done)

	time.Sleep(100 * time.Millisecond)

	if got := attempts.Load(); got != int32(saveRetryCount) {
		t.Errorf("save attempts = %d, want %d", got, saveRetryCount)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(notified) != 0 {
		t.Errorf("OnSnapshot callback: got %d calls, want 0 (all retries failed)", len(notified))
	}
}

func TestScanExistingFiles_NewDirectory(t *testing.T) {
	watchDir := t.TempDir()

	var mu sync.Mutex
	var saved []string

	saver := func(path string, content []byte, maxSnapshots int) (bool, error) {
		mu.Lock()
		saved = append(saved, path)
		mu.Unlock()
		return true, nil
	}

	cfg := newTestConfig(watchDir, []string{".go", ".txt"}, []string{}, 1, 1048576)

	w, err := New(cfg, saver)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer w.Close()

	done := make(chan struct{})
	go w.Run(done)

	// Prepare a directory with files outside the watch tree
	srcDir := t.TempDir()
	subDir := filepath.Join(srcDir, "sub")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := range 5 {
		f := filepath.Join(srcDir, fmt.Sprintf("file%d.go", i))
		if err := os.WriteFile(f, []byte(fmt.Sprintf("package f%d", i)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for i := range 3 {
		f := filepath.Join(subDir, fmt.Sprintf("sub%d.txt", i))
		if err := os.WriteFile(f, []byte(fmt.Sprintf("sub content %d", i)), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Move the prepared directory into the watch tree (triggers Create event)
	destDir := filepath.Join(watchDir, "newproject")
	if err := os.Rename(srcDir, destDir); err != nil {
		t.Fatal(err)
	}

	// Wait for debounce + scan to complete
	time.Sleep(3 * time.Second)
	close(done)

	mu.Lock()
	defer mu.Unlock()

	// All 8 files (5 .go + 3 .txt) should be saved
	if len(saved) < 8 {
		t.Errorf("scan new directory: got %d saves, want at least 8", len(saved))
	}
}

func TestScanExistingFiles_RespectsFilters(t *testing.T) {
	watchDir := t.TempDir()

	var mu sync.Mutex
	var saved []string

	saver := func(path string, content []byte, maxSnapshots int) (bool, error) {
		mu.Lock()
		saved = append(saved, path)
		mu.Unlock()
		return true, nil
	}

	cfg := newTestConfig(watchDir, []string{".go"}, []string{"**/vendor/**"}, 1, 100)

	w, err := New(cfg, saver)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer w.Close()

	done := make(chan struct{})
	go w.Run(done)

	// Prepare directory with various files
	srcDir := t.TempDir()

	// Trackable file
	if err := os.WriteFile(filepath.Join(srcDir, "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Wrong extension — should be excluded
	if err := os.WriteFile(filepath.Join(srcDir, "readme.md"), []byte("# readme"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Excluded directory
	vendorDir := filepath.Join(srcDir, "vendor")
	if err := os.MkdirAll(vendorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vendorDir, "lib.go"), []byte("package lib"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Binary file with .go extension
	if err := os.WriteFile(filepath.Join(srcDir, "binary.go"), []byte{0x89, 0x50, 0x00, 0x4E}, 0o644); err != nil {
		t.Fatal(err)
	}
	// Oversized file
	bigContent := make([]byte, 200)
	for i := range bigContent {
		bigContent[i] = 'x'
	}
	if err := os.WriteFile(filepath.Join(srcDir, "big.go"), bigContent, 0o644); err != nil {
		t.Fatal(err)
	}

	// Move into watch tree
	destDir := filepath.Join(watchDir, "filtered")
	if err := os.Rename(srcDir, destDir); err != nil {
		t.Fatal(err)
	}

	time.Sleep(3 * time.Second)
	close(done)

	mu.Lock()
	defer mu.Unlock()

	// Only main.go should be saved (correct ext, not excluded, not binary, not oversized)
	if len(saved) != 1 {
		t.Errorf("filtered scan: got %d saves, want 1", len(saved))
		for _, s := range saved {
			t.Logf("  saved: %s", s)
		}
	}
	if len(saved) == 1 && filepath.Base(saved[0]) != "main.go" {
		t.Errorf("saved file = %s, want main.go", filepath.Base(saved[0]))
	}
}

func TestScanExistingFiles_NoDuplicateScan(t *testing.T) {
	watchDir := t.TempDir()
	dir := t.TempDir()

	// Create some files in the directory
	for i := range 3 {
		f := filepath.Join(dir, fmt.Sprintf("file%d.go", i))
		if err := os.WriteFile(f, []byte(fmt.Sprintf("package f%d", i)), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var scanCount atomic.Int32

	saver := func(path string, content []byte, maxSnapshots int) (bool, error) {
		scanCount.Add(1)
		return true, nil
	}

	cfg := newTestConfig(watchDir, []string{".go"}, []string{}, 1, 1048576)

	w, err := New(cfg, saver)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer w.Close()

	done := make(chan struct{})
	go w.Run(done)

	// Pre-register the directory as scanning to verify duplicate rejection
	if !w.tryStartScan(dir) {
		t.Fatal("tryStartScan should succeed on first call")
	}

	// Second call should be rejected while first is active
	w.scanExistingFiles(dir)

	// Wait briefly for save worker
	time.Sleep(200 * time.Millisecond)

	got := scanCount.Load()
	if got != 0 {
		t.Errorf("duplicate scan: got %d saves, want 0 (scan should be skipped)", got)
	}

	// Clean up the pre-registered entry
	w.finishScan(dir)

	// Now a real scan should work
	// Note: dir is outside the WatchSet dirs, so shouldTrack will return false.
	// We need to scan a dir inside the WatchSet for files to be tracked.
	innerDir := filepath.Join(watchDir, "inner")
	if err := os.MkdirAll(innerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		f := filepath.Join(innerDir, fmt.Sprintf("file%d.go", i))
		if err := os.WriteFile(f, []byte(fmt.Sprintf("package f%d", i)), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	w.scanExistingFiles(innerDir)

	time.Sleep(500 * time.Millisecond)
	close(done)

	got = scanCount.Load()
	if got != 3 {
		t.Errorf("after finish: got %d saves, want 3", got)
	}
}

func TestSaveQueue_SerializesWrites(t *testing.T) {
	dir := t.TempDir()

	var concurrent atomic.Int32
	var maxConcurrent atomic.Int32
	var savedCount atomic.Int32

	saver := func(path string, content []byte, maxSnapshots int) (bool, error) {
		c := concurrent.Add(1)
		defer concurrent.Add(-1)
		// Track max concurrency
		for {
			cur := maxConcurrent.Load()
			if c <= cur || maxConcurrent.CompareAndSwap(cur, c) {
				break
			}
		}
		// Simulate slow DB write
		time.Sleep(10 * time.Millisecond)
		savedCount.Add(1)
		return true, nil
	}

	cfg := newTestConfig(dir, []string{".txt"}, []string{}, 1, 1048576)

	w, err := New(cfg, saver)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer w.Close()

	done := make(chan struct{})
	go w.Run(done)

	// Create 50 files simultaneously
	fileCount := 50
	for i := range fileCount {
		f := filepath.Join(dir, fmt.Sprintf("file%d.txt", i))
		if err := os.WriteFile(f, []byte(fmt.Sprintf("content %d", i)), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Wait for debounce + all saves to complete
	time.Sleep(4 * time.Second)
	close(done)

	time.Sleep(200 * time.Millisecond)

	if got := maxConcurrent.Load(); got != 1 {
		t.Errorf("max concurrent saves = %d, want 1 (serialized)", got)
	}
	if got := savedCount.Load(); got != int32(fileCount) {
		t.Errorf("saved count = %d, want %d", got, fileCount)
	}
}

// Tests for WatchSet-specific features

func TestFindWatchSet_LongestPrefixMatch(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := filepath.Join(dir1, "subdir")
	if err := os.MkdirAll(dir2, 0o755); err != nil {
		t.Fatal(err)
	}

	cfg := Config{
		WatchSets: []config.WatchSet{
			{
				Name:        "parent",
				Dirs:        []string{dir1},
				DebounceSec: 1,
				MaxFileSize: 1048576,
			},
			{
				Name:        "child",
				Dirs:        []string{dir2},
				DebounceSec: 1,
				MaxFileSize: 1048576,
			},
		},
	}

	w, err := New(cfg, func(path string, content []byte, maxSnapshots int) (bool, error) {
		return true, nil
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer w.Close()

	// File in subdir should match "child" (longest prefix)
	ws := w.findWatchSet(filepath.Join(dir2, "test.go"))
	if ws == nil {
		t.Fatal("findWatchSet returned nil for file in child dir")
	}
	if ws.name != "child" {
		t.Errorf("findWatchSet returned %q, want %q", ws.name, "child")
	}

	// File directly in parent dir should match "parent"
	ws = w.findWatchSet(filepath.Join(dir1, "test.go"))
	if ws == nil {
		t.Fatal("findWatchSet returned nil for file in parent dir")
	}
	if ws.name != "parent" {
		t.Errorf("findWatchSet returned %q, want %q", ws.name, "parent")
	}

	// File outside both dirs should return nil
	ws = w.findWatchSet("/some/other/dir/test.go")
	if ws != nil {
		t.Errorf("findWatchSet returned %q for file outside all WatchSets, want nil", ws.name)
	}
}

func TestFindWatchSet_RootDirectory(t *testing.T) {
	dir := t.TempDir()
	cfg := newTestConfig(dir, nil, nil, 1, 1048576)
	w, err := New(cfg, func(path string, content []byte, maxSnapshots int) (bool, error) {
		return true, nil
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer w.Close()

	// The root directory itself should match
	ws := w.findWatchSet(dir)
	if ws == nil {
		t.Fatal("findWatchSet returned nil for root directory itself")
	}
	if ws.name != "test" {
		t.Errorf("findWatchSet returned %q, want %q", ws.name, "test")
	}
}

func TestMultipleWatchSets_DifferentExtensions(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	cfg := Config{
		WatchSets: []config.WatchSet{
			{
				Name:        "go-project",
				Dirs:        []string{dir1},
				Extensions:  []string{".go"},
				DebounceSec: 1,
				MaxFileSize: 1048576,
			},
			{
				Name:        "web-project",
				Dirs:        []string{dir2},
				Extensions:  []string{".ts", ".tsx"},
				DebounceSec: 1,
				MaxFileSize: 1048576,
			},
		},
	}

	w, err := New(cfg, func(path string, content []byte, maxSnapshots int) (bool, error) {
		return true, nil
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer w.Close()

	// .go in dir1 should be tracked
	if !w.shouldTrack(filepath.Join(dir1, "main.go")) {
		t.Error("shouldTrack(.go in go-project) = false, want true")
	}
	// .ts in dir1 should NOT be tracked (not in go-project's extensions)
	if w.shouldTrack(filepath.Join(dir1, "app.ts")) {
		t.Error("shouldTrack(.ts in go-project) = true, want false")
	}
	// .ts in dir2 should be tracked
	if !w.shouldTrack(filepath.Join(dir2, "app.ts")) {
		t.Error("shouldTrack(.ts in web-project) = false, want true")
	}
	// .go in dir2 should NOT be tracked
	if w.shouldTrack(filepath.Join(dir2, "main.go")) {
		t.Error("shouldTrack(.go in web-project) = true, want false")
	}
}

func TestMultipleWatchSets_DifferentExcludePatterns(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	cfg := Config{
		WatchSets: []config.WatchSet{
			{
				Name:            "project-a",
				Dirs:            []string{dir1},
				ExcludePatterns: []string{"**/node_modules/**"},
				DebounceSec:     1,
				MaxFileSize:     1048576,
			},
			{
				Name:            "project-b",
				Dirs:            []string{dir2},
				ExcludePatterns: []string{"**/vendor/**"},
				DebounceSec:     1,
				MaxFileSize:     1048576,
			},
		},
	}

	w, err := New(cfg, func(path string, content []byte, maxSnapshots int) (bool, error) {
		return true, nil
	})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer w.Close()

	// node_modules in project-a should be excluded
	if !w.isExcluded(filepath.Join(dir1, "node_modules", "pkg")) {
		t.Error("isExcluded(node_modules in project-a) = false, want true")
	}
	// node_modules in project-b should NOT be excluded (project-b excludes vendor, not node_modules)
	if w.isExcluded(filepath.Join(dir2, "node_modules", "pkg")) {
		t.Error("isExcluded(node_modules in project-b) = true, want false")
	}
	// vendor in project-b should be excluded
	if !w.isExcluded(filepath.Join(dir2, "vendor", "lib")) {
		t.Error("isExcluded(vendor in project-b) = false, want true")
	}
	// vendor in project-a should NOT be excluded
	if w.isExcluded(filepath.Join(dir1, "vendor", "lib")) {
		t.Error("isExcluded(vendor in project-a) = true, want false")
	}
}

func TestMultipleWatchSets_MaxSnapshotsPassedToSaver(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	var mu sync.Mutex
	var capturedMaxSnapshots []int

	saver := func(path string, content []byte, maxSnapshots int) (bool, error) {
		mu.Lock()
		capturedMaxSnapshots = append(capturedMaxSnapshots, maxSnapshots)
		mu.Unlock()
		return true, nil
	}

	cfg := Config{
		WatchSets: []config.WatchSet{
			{
				Name:         "limited",
				Dirs:         []string{dir1},
				Extensions:   []string{".txt"},
				DebounceSec:  1,
				MaxFileSize:  1048576,
				MaxSnapshots: 5,
			},
			{
				Name:         "unlimited",
				Dirs:         []string{dir2},
				Extensions:   []string{".txt"},
				DebounceSec:  1,
				MaxFileSize:  1048576,
				MaxSnapshots: 0,
			},
		},
	}

	w, err := New(cfg, saver)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer w.Close()

	done := make(chan struct{})
	go w.Run(done)

	// Write to dir1 (maxSnapshots=5)
	if err := os.WriteFile(filepath.Join(dir1, "file.txt"), []byte("content1"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Write to dir2 (maxSnapshots=0)
	if err := os.WriteFile(filepath.Join(dir2, "file.txt"), []byte("content2"), 0o644); err != nil {
		t.Fatal(err)
	}

	time.Sleep(3 * time.Second)
	close(done)

	mu.Lock()
	defer mu.Unlock()

	if len(capturedMaxSnapshots) != 2 {
		t.Fatalf("expected 2 saves, got %d", len(capturedMaxSnapshots))
	}

	// Check that both maxSnapshots values were captured (order may vary)
	has5 := false
	has0 := false
	for _, ms := range capturedMaxSnapshots {
		if ms == 5 {
			has5 = true
		}
		if ms == 0 {
			has0 = true
		}
	}
	if !has5 {
		t.Error("expected maxSnapshots=5 to be captured for dir1")
	}
	if !has0 {
		t.Error("expected maxSnapshots=0 to be captured for dir2")
	}
}

func TestMultipleWatchSets_DifferentDebounceSec(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	var mu sync.Mutex
	savedTimes := make(map[string]time.Time)
	writeTime := time.Now()

	saver := func(path string, content []byte, maxSnapshots int) (bool, error) {
		mu.Lock()
		savedTimes[path] = time.Now()
		mu.Unlock()
		return true, nil
	}

	cfg := Config{
		WatchSets: []config.WatchSet{
			{
				Name:        "fast",
				Dirs:        []string{dir1},
				Extensions:  []string{".txt"},
				DebounceSec: 1,
				MaxFileSize: 1048576,
			},
			{
				Name:        "slow",
				Dirs:        []string{dir2},
				Extensions:  []string{".txt"},
				DebounceSec: 3,
				MaxFileSize: 1048576,
			},
		},
	}

	w, err := New(cfg, saver)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer w.Close()

	done := make(chan struct{})
	go w.Run(done)

	fastFile := filepath.Join(dir1, "fast.txt")
	slowFile := filepath.Join(dir2, "slow.txt")
	writeTime = time.Now()
	if err := os.WriteFile(fastFile, []byte("fast"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(slowFile, []byte("slow"), 0o644); err != nil {
		t.Fatal(err)
	}

	// After 2 seconds: fast should be saved, slow should not
	time.Sleep(2 * time.Second)

	mu.Lock()
	_, fastSaved := savedTimes[fastFile]
	_, slowSaved := savedTimes[slowFile]
	mu.Unlock()

	if !fastSaved {
		t.Error("fast file (1s debounce) should be saved after 2s")
	}
	if slowSaved {
		t.Error("slow file (3s debounce) should NOT be saved after 2s")
	}

	// After 4 seconds total: slow should also be saved
	time.Sleep(2 * time.Second)
	close(done)

	mu.Lock()
	defer mu.Unlock()
	_, slowSaved = savedTimes[slowFile]
	if !slowSaved {
		t.Error("slow file (3s debounce) should be saved after 4s total")
	}

	// Verify timing: fast saved before slow
	if savedTimes[fastFile].After(savedTimes[slowFile]) {
		t.Error("fast file should have been saved before slow file")
	}

	_ = writeTime // avoid unused variable error
}

func TestMultipleWatchSets_DifferentMaxFileSize(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()

	var mu sync.Mutex
	var saved []string

	saver := func(path string, content []byte, maxSnapshots int) (bool, error) {
		mu.Lock()
		saved = append(saved, path)
		mu.Unlock()
		return true, nil
	}

	cfg := Config{
		WatchSets: []config.WatchSet{
			{
				Name:        "small-limit",
				Dirs:        []string{dir1},
				Extensions:  []string{".txt"},
				DebounceSec: 1,
				MaxFileSize: 50, // 50 bytes
			},
			{
				Name:        "large-limit",
				Dirs:        []string{dir2},
				Extensions:  []string{".txt"},
				DebounceSec: 1,
				MaxFileSize: 500, // 500 bytes
			},
		},
	}

	w, err := New(cfg, saver)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer w.Close()

	done := make(chan struct{})
	go w.Run(done)

	// Write a 100-byte file to both dirs
	content := make([]byte, 100)
	for i := range content {
		content[i] = 'x'
	}

	if err := os.WriteFile(filepath.Join(dir1, "file.txt"), content, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir2, "file.txt"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	time.Sleep(3 * time.Second)
	close(done)

	mu.Lock()
	defer mu.Unlock()

	// Only file in dir2 should be saved (100 bytes > 50 limit in dir1, but < 500 limit in dir2)
	if len(saved) != 1 {
		t.Errorf("expected 1 save, got %d", len(saved))
		for _, s := range saved {
			t.Logf("  saved: %s", s)
		}
	}
	if len(saved) == 1 && saved[0] != filepath.Join(dir2, "file.txt") {
		t.Errorf("saved file = %s, want %s", saved[0], filepath.Join(dir2, "file.txt"))
	}
}
