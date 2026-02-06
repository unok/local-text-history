package watcher

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestShouldTrack_Extensions(t *testing.T) {
	w := &Watcher{
		config: Config{
			Extensions:      []string{".go", ".ts"},
			ExcludePatterns: []string{},
		},
		extSet: map[string]struct{}{".go": {}, ".ts": {}},
	}

	tests := []struct {
		path string
		want bool
	}{
		{"/tmp/main.go", true},
		{"/tmp/app.ts", true},
		{"/tmp/readme.md", false},
		{"/tmp/image.png", false},
		{"/tmp/noext", false},
	}

	for _, tt := range tests {
		got := w.shouldTrack(tt.path)
		if got != tt.want {
			t.Errorf("shouldTrack(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestShouldTrack_NoExtensions(t *testing.T) {
	w := &Watcher{
		config: Config{
			Extensions:      nil,
			ExcludePatterns: []string{"**/node_modules/**"},
		},
		extSet: map[string]struct{}{},
	}

	tests := []struct {
		path string
		want bool
	}{
		{"/tmp/main.go", true},
		{"/tmp/app.ts", true},
		{"/tmp/readme.md", true},
		{"/tmp/image.png", true},
		{"/tmp/noext", true},
		{"/tmp/node_modules/pkg/index.js", false},
	}

	for _, tt := range tests {
		got := w.shouldTrack(tt.path)
		if got != tt.want {
			t.Errorf("shouldTrack(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestIsExcluded(t *testing.T) {
	w := &Watcher{
		config: Config{
			ExcludePatterns: []string{
				"**/node_modules/**",
				"**/.git/**",
				"**/*.min.js",
			},
		},
	}

	tests := []struct {
		path string
		want bool
	}{
		{"/home/user/project/node_modules/pkg/index.js", true},
		{"/home/user/project/.git/objects/abc", true},
		{"/home/user/project/src/app.min.js", true},
		{"/home/user/project/src/main.go", false},
		{"/home/user/project/src/app.js", false},
	}

	for _, tt := range tests {
		got := w.isExcluded(tt.path)
		if got != tt.want {
			t.Errorf("isExcluded(%q) = %v, want %v", tt.path, got, tt.want)
		}
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

	saver := func(path string, content []byte) (bool, error) {
		mu.Lock()
		saved = append(saved, path)
		mu.Unlock()
		return true, nil
	}

	cfg := Config{
		WatchDirs:       []string{dir},
		Extensions:      []string{".txt"},
		ExcludePatterns: []string{},
		DebounceSec:     1,
		MaxFileSize:     1048576,
	}

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

	saver := func(path string, content []byte) (bool, error) {
		mu.Lock()
		saved = append(saved, path)
		mu.Unlock()
		return true, nil
	}

	cfg := Config{
		WatchDirs:       []string{dir},
		Extensions:      []string{".txt"},
		ExcludePatterns: []string{},
		DebounceSec:     1,
		MaxFileSize:     100, // 100 bytes max
	}

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

	saver := func(path string, content []byte) (bool, error) {
		mu.Lock()
		saved = append(saved, path)
		mu.Unlock()
		return true, nil
	}

	cfg := Config{
		WatchDirs:       []string{dir},
		Extensions:      []string{".js"},
		ExcludePatterns: []string{"**/node_modules/**"},
		DebounceSec:     1,
		MaxFileSize:     1048576,
	}

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

	saver := func(path string, content []byte) (bool, error) {
		mu.Lock()
		saved = append(saved, path)
		mu.Unlock()
		return true, nil
	}

	cfg := Config{
		WatchDirs:       []string{dir},
		Extensions:      []string{".txt"},
		ExcludePatterns: []string{},
		DebounceSec:     1,
		MaxFileSize:     1048576,
	}

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

	saver := func(path string, content []byte) (bool, error) {
		mu.Lock()
		saved = append(saved, path)
		mu.Unlock()
		return true, nil
	}

	cfg := Config{
		WatchDirs:       []string{dir},
		Extensions:      []string{".txt"},
		ExcludePatterns: []string{},
		DebounceSec:     1,
		MaxFileSize:     1048576,
	}

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

	saver := func(path string, content []byte) (bool, error) {
		mu.Lock()
		saved = append(saved, path)
		mu.Unlock()
		return true, nil
	}

	cfg := Config{
		WatchDirs:       []string{dir},
		Extensions:      nil, // No extension filter
		ExcludePatterns: []string{},
		DebounceSec:     1,
		MaxFileSize:     1048576,
	}

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

	saver := func(path string, content []byte) (bool, error) {
		return true, nil
	}

	cfg := Config{
		WatchDirs:       []string{dir},
		Extensions:      []string{".txt"},
		ExcludePatterns: []string{},
		DebounceSec:     1,
		MaxFileSize:     1048576,
	}

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

	saver := func(path string, content []byte) (bool, error) {
		saveMu.Lock()
		saveCount++
		first := saveCount == 1
		saveMu.Unlock()
		// First call saves, second is a duplicate
		return first, nil
	}

	var mu sync.Mutex
	var notified []string

	cfg := Config{
		WatchDirs:       []string{dir},
		Extensions:      []string{".txt"},
		ExcludePatterns: []string{},
		DebounceSec:     1,
		MaxFileSize:     1048576,
	}

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
