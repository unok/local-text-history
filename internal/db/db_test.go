package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

func newTestDB(t *testing.T, maxSnapshots int) *DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := New(dbPath, maxSnapshots)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}

func TestSaveSnapshot_Basic(t *testing.T) {
	d := newTestDB(t, 0)

	saved, err := d.SaveSnapshot("/tmp/test.go", []byte("package main"))
	if err != nil {
		t.Fatalf("SaveSnapshot() error: %v", err)
	}
	if !saved {
		t.Error("SaveSnapshot() = false, want true")
	}

	files, err := d.SearchFiles("test.go", 10, 0)
	if err != nil {
		t.Fatalf("SearchFiles() error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("SearchFiles() returned %d files, want 1", len(files))
	}
	if files[0].Path != "/tmp/test.go" {
		t.Errorf("Path = %s, want /tmp/test.go", files[0].Path)
	}
}

func TestSaveSnapshot_DuplicateSkip(t *testing.T) {
	d := newTestDB(t, 0)
	content := []byte("package main")

	saved, err := d.SaveSnapshot("/tmp/test.go", content)
	if err != nil {
		t.Fatalf("first SaveSnapshot() error: %v", err)
	}
	if !saved {
		t.Error("first SaveSnapshot() = false, want true")
	}

	saved, err = d.SaveSnapshot("/tmp/test.go", content)
	if err != nil {
		t.Fatalf("second SaveSnapshot() error: %v", err)
	}
	if saved {
		t.Error("second SaveSnapshot() = true, want false (duplicate)")
	}

	files, err := d.SearchFiles("test.go", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	snapshots, err := d.GetSnapshots(files[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != 1 {
		t.Errorf("got %d snapshots, want 1", len(snapshots))
	}
}

func TestSaveSnapshot_DifferentContent(t *testing.T) {
	d := newTestDB(t, 0)

	if _, err := d.SaveSnapshot("/tmp/test.go", []byte("v1")); err != nil {
		t.Fatal(err)
	}
	if _, err := d.SaveSnapshot("/tmp/test.go", []byte("v2")); err != nil {
		t.Fatal(err)
	}

	files, err := d.SearchFiles("test.go", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	snapshots, err := d.GetSnapshots(files[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != 2 {
		t.Errorf("got %d snapshots, want 2", len(snapshots))
	}
}

func TestZstdRoundTrip(t *testing.T) {
	d := newTestDB(t, 0)
	original := []byte("Hello, zstd compression test content!")

	if _, err := d.SaveSnapshot("/tmp/zstd.txt", original); err != nil {
		t.Fatal(err)
	}

	files, err := d.SearchFiles("zstd.txt", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	snapshots, err := d.GetSnapshots(files[0].ID)
	if err != nil {
		t.Fatal(err)
	}

	snap, err := d.GetSnapshot(snapshots[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(snap.Content) != string(original) {
		t.Errorf("decompressed content = %q, want %q", snap.Content, original)
	}
	if snap.Size != int64(len(original)) {
		t.Errorf("Size = %d, want %d", snap.Size, len(original))
	}
}

func TestMaxSnapshots(t *testing.T) {
	d := newTestDB(t, 3)

	for i := range 5 {
		content := []byte(fmt.Sprintf("version %d", i))
		if _, err := d.SaveSnapshot("/tmp/max.go", content); err != nil {
			t.Fatal(err)
		}
	}

	files, err := d.SearchFiles("max.go", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	snapshots, err := d.GetSnapshots(files[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != 3 {
		t.Errorf("got %d snapshots, want 3 (maxSnapshots limit)", len(snapshots))
	}
}

func TestGetFile(t *testing.T) {
	d := newTestDB(t, 0)

	if _, err := d.SaveSnapshot("/tmp/getfile.go", []byte("content")); err != nil {
		t.Fatal(err)
	}

	files, err := d.SearchFiles("getfile.go", 10, 0)
	if err != nil {
		t.Fatal(err)
	}

	file, err := d.GetFile(files[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if file.Path != "/tmp/getfile.go" {
		t.Errorf("Path = %s, want /tmp/getfile.go", file.Path)
	}
}

func TestGetFile_NotFound(t *testing.T) {
	d := newTestDB(t, 0)

	_, err := d.GetFile("00000000-0000-0000-0000-000000000000")
	if err == nil {
		t.Fatal("GetFile() should error on non-existent ID")
	}
}

func TestDeleteFile(t *testing.T) {
	d := newTestDB(t, 0)

	if _, err := d.SaveSnapshot("/tmp/delete.go", []byte("content")); err != nil {
		t.Fatal(err)
	}

	files, err := d.SearchFiles("delete.go", 10, 0)
	if err != nil {
		t.Fatal(err)
	}

	if err := d.DeleteFile(files[0].ID); err != nil {
		t.Fatalf("DeleteFile() error: %v", err)
	}

	_, err = d.GetFile(files[0].ID)
	if err == nil {
		t.Error("GetFile() should error after deletion")
	}
}

func TestDeleteFile_NotFound(t *testing.T) {
	d := newTestDB(t, 0)

	err := d.DeleteFile("00000000-0000-0000-0000-000000000000")
	if err == nil {
		t.Fatal("DeleteFile() should error on non-existent ID")
	}
}

func TestGetStats_Empty(t *testing.T) {
	d := newTestDB(t, 0)

	stats, err := d.GetStats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalFiles != 0 {
		t.Errorf("TotalFiles = %d, want 0", stats.TotalFiles)
	}
	if stats.TotalSnapshots != 0 {
		t.Errorf("TotalSnapshots = %d, want 0", stats.TotalSnapshots)
	}
}

func TestGetStats_WithData(t *testing.T) {
	d := newTestDB(t, 0)

	if _, err := d.SaveSnapshot("/tmp/a.go", []byte("aa")); err != nil {
		t.Fatal(err)
	}
	if _, err := d.SaveSnapshot("/tmp/b.go", []byte("bbb")); err != nil {
		t.Fatal(err)
	}

	stats, err := d.GetStats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalFiles != 2 {
		t.Errorf("TotalFiles = %d, want 2", stats.TotalFiles)
	}
	if stats.TotalSnapshots != 2 {
		t.Errorf("TotalSnapshots = %d, want 2", stats.TotalSnapshots)
	}
	if stats.TotalSize != 5 {
		t.Errorf("TotalSize = %d, want 5", stats.TotalSize)
	}
}

func TestSearchFiles_Pagination(t *testing.T) {
	d := newTestDB(t, 0)

	for i := range 5 {
		path := fmt.Sprintf("/tmp/search%d.go", i)
		if _, err := d.SaveSnapshot(path, []byte("content")); err != nil {
			t.Fatal(err)
		}
	}

	files, err := d.SearchFiles("search", 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Errorf("page 1: got %d files, want 2", len(files))
	}

	files, err = d.SearchFiles("search", 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Errorf("page 2: got %d files, want 2", len(files))
	}

	files, err = d.SearchFiles("search", 2, 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Errorf("page 3: got %d files, want 1", len(files))
	}
}

func TestGetRecentSnapshots_Empty(t *testing.T) {
	d := newTestDB(t, 0)

	entries, err := d.GetRecentSnapshots(50)
	if err != nil {
		t.Fatalf("GetRecentSnapshots() error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries, want 0", len(entries))
	}
}

func TestGetRecentSnapshots_WithData(t *testing.T) {
	d := newTestDB(t, 0)

	if _, err := d.SaveSnapshot("/tmp/a.go", []byte("aaa")); err != nil {
		t.Fatal(err)
	}
	if _, err := d.SaveSnapshot("/tmp/b.go", []byte("bbb")); err != nil {
		t.Fatal(err)
	}
	if _, err := d.SaveSnapshot("/tmp/a.go", []byte("aaa-v2")); err != nil {
		t.Fatal(err)
	}

	entries, err := d.GetRecentSnapshots(50)
	if err != nil {
		t.Fatalf("GetRecentSnapshots() error: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("got %d entries, want 3", len(entries))
	}

	// Most recent first: a.go v2, b.go, a.go v1
	if entries[0].FilePath != "/tmp/a.go" {
		t.Errorf("entries[0].FilePath = %s, want /tmp/a.go", entries[0].FilePath)
	}
	if entries[1].FilePath != "/tmp/b.go" {
		t.Errorf("entries[1].FilePath = %s, want /tmp/b.go", entries[1].FilePath)
	}
	if entries[2].FilePath != "/tmp/a.go" {
		t.Errorf("entries[2].FilePath = %s, want /tmp/a.go", entries[2].FilePath)
	}

	// Verify all fields are populated
	for i, e := range entries {
		if e.SnapshotID == "" {
			t.Errorf("entries[%d].SnapshotID is empty", i)
		}
		if e.FileID == "" {
			t.Errorf("entries[%d].FileID is empty", i)
		}
		if e.Size == 0 {
			t.Errorf("entries[%d].Size is 0", i)
		}
		if e.Hash == "" {
			t.Errorf("entries[%d].Hash is empty", i)
		}
		if e.Timestamp == 0 {
			t.Errorf("entries[%d].Timestamp is 0", i)
		}
	}
}

func TestGetRecentSnapshots_Limit(t *testing.T) {
	d := newTestDB(t, 0)

	for i := range 5 {
		content := []byte(fmt.Sprintf("content-%d", i))
		path := fmt.Sprintf("/tmp/limit%d.go", i)
		if _, err := d.SaveSnapshot(path, content); err != nil {
			t.Fatal(err)
		}
	}

	entries, err := d.GetRecentSnapshots(3)
	if err != nil {
		t.Fatalf("GetRecentSnapshots() error: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("got %d entries, want 3", len(entries))
	}
}

func TestUUIDv7_Generation(t *testing.T) {
	d := newTestDB(t, 0)

	if _, err := d.SaveSnapshot("/tmp/uuid.go", []byte("content")); err != nil {
		t.Fatal(err)
	}

	files, err := d.SearchFiles("uuid.go", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}

	// Verify file ID is a valid UUID
	fileID := files[0].ID
	parsed, err := uuid.Parse(fileID)
	if err != nil {
		t.Fatalf("file ID %q is not a valid UUID: %v", fileID, err)
	}
	if parsed.Version() != 7 {
		t.Errorf("file ID UUID version = %d, want 7", parsed.Version())
	}

	// Verify snapshot ID is a valid UUIDv7
	snapshots, err := d.GetSnapshots(fileID)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != 1 {
		t.Fatalf("got %d snapshots, want 1", len(snapshots))
	}

	snapID := snapshots[0].ID
	parsedSnap, err := uuid.Parse(snapID)
	if err != nil {
		t.Fatalf("snapshot ID %q is not a valid UUID: %v", snapID, err)
	}
	if parsedSnap.Version() != 7 {
		t.Errorf("snapshot ID UUID version = %d, want 7", parsedSnap.Version())
	}

	// Verify GetSnapshot also returns valid UUIDv7
	snap, err := d.GetSnapshot(snapID)
	if err != nil {
		t.Fatal(err)
	}
	if snap.ID != snapID {
		t.Errorf("GetSnapshot ID = %s, want %s", snap.ID, snapID)
	}
	if snap.FileID != fileID {
		t.Errorf("GetSnapshot FileID = %s, want %s", snap.FileID, fileID)
	}
}

// createOldSchemaDB creates a database with the old INTEGER PRIMARY KEY schema
// and inserts test data for migration testing.
func createOldSchemaDB(t *testing.T, dbPath string) {
	t.Helper()
	sqlDB, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on")
	if err != nil {
		t.Fatalf("opening old schema DB: %v", err)
	}
	defer sqlDB.Close()

	oldSchema := `
	CREATE TABLE files (
		id       INTEGER PRIMARY KEY AUTOINCREMENT,
		path     TEXT NOT NULL UNIQUE,
		created  INTEGER NOT NULL DEFAULT (unixepoch()),
		updated  INTEGER NOT NULL DEFAULT (unixepoch())
	);
	CREATE TABLE snapshots (
		id        INTEGER PRIMARY KEY AUTOINCREMENT,
		file_id   INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
		content   BLOB NOT NULL,
		size      INTEGER NOT NULL,
		hash      TEXT NOT NULL,
		timestamp INTEGER NOT NULL DEFAULT (unixepoch())
	);
	CREATE INDEX idx_snapshots_file_ts ON snapshots(file_id, timestamp DESC);
	CREATE INDEX idx_snapshots_timestamp ON snapshots(timestamp DESC, id DESC);
	CREATE INDEX idx_files_path ON files(path);
	`
	if _, err := sqlDB.Exec(oldSchema); err != nil {
		t.Fatalf("creating old schema: %v", err)
	}

	// Insert test files
	if _, err := sqlDB.Exec(
		"INSERT INTO files (id, path, created, updated) VALUES (1, '/tmp/old1.go', 1000, 2000)",
	); err != nil {
		t.Fatalf("inserting file 1: %v", err)
	}
	if _, err := sqlDB.Exec(
		"INSERT INTO files (id, path, created, updated) VALUES (2, '/tmp/old2.go', 1100, 2100)",
	); err != nil {
		t.Fatalf("inserting file 2: %v", err)
	}

	// Insert test snapshots (content is raw bytes for simplicity since
	// we're testing migration, not compression)
	if _, err := sqlDB.Exec(
		"INSERT INTO snapshots (id, file_id, content, size, hash, timestamp) VALUES (1, 1, X'68656C6C6F', 5, 'hash1', 1000)",
	); err != nil {
		t.Fatalf("inserting snapshot 1: %v", err)
	}
	if _, err := sqlDB.Exec(
		"INSERT INTO snapshots (id, file_id, content, size, hash, timestamp) VALUES (2, 1, X'776F726C64', 5, 'hash2', 2000)",
	); err != nil {
		t.Fatalf("inserting snapshot 2: %v", err)
	}
	if _, err := sqlDB.Exec(
		"INSERT INTO snapshots (id, file_id, content, size, hash, timestamp) VALUES (3, 2, X'746573743131', 6, 'hash3', 1100)",
	); err != nil {
		t.Fatalf("inserting snapshot 3: %v", err)
	}
}

func TestMigrateIfNeeded_OldSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migrate.db")

	// Create DB with old INTEGER schema and seed data
	createOldSchemaDB(t, dbPath)

	// Open with New(), which should trigger migration
	d, err := New(dbPath, 0)
	if err != nil {
		t.Fatalf("New() after migration error: %v", err)
	}
	defer d.Close()

	// Verify files were migrated with UUIDv7 IDs
	files1, err := d.SearchFiles("old1.go", 10, 0)
	if err != nil {
		t.Fatalf("SearchFiles(old1): %v", err)
	}
	if len(files1) != 1 {
		t.Fatalf("got %d files for old1.go, want 1", len(files1))
	}
	parsed1, err := uuid.Parse(files1[0].ID)
	if err != nil {
		t.Fatalf("file1 ID %q is not valid UUID: %v", files1[0].ID, err)
	}
	if parsed1.Version() != 7 {
		t.Errorf("file1 UUID version = %d, want 7", parsed1.Version())
	}
	if files1[0].Path != "/tmp/old1.go" {
		t.Errorf("file1 Path = %s, want /tmp/old1.go", files1[0].Path)
	}
	if files1[0].Created != 1000 {
		t.Errorf("file1 Created = %d, want 1000", files1[0].Created)
	}
	if files1[0].Updated != 2000 {
		t.Errorf("file1 Updated = %d, want 2000", files1[0].Updated)
	}

	files2, err := d.SearchFiles("old2.go", 10, 0)
	if err != nil {
		t.Fatalf("SearchFiles(old2): %v", err)
	}
	if len(files2) != 1 {
		t.Fatalf("got %d files for old2.go, want 1", len(files2))
	}
	parsed2, err := uuid.Parse(files2[0].ID)
	if err != nil {
		t.Fatalf("file2 ID %q is not valid UUID: %v", files2[0].ID, err)
	}
	if parsed2.Version() != 7 {
		t.Errorf("file2 UUID version = %d, want 7", parsed2.Version())
	}

	// Verify snapshots were migrated with correct file_id references
	snapshots1, err := d.GetSnapshots(files1[0].ID)
	if err != nil {
		t.Fatalf("GetSnapshots(file1): %v", err)
	}
	if len(snapshots1) != 2 {
		t.Fatalf("got %d snapshots for file1, want 2", len(snapshots1))
	}
	for _, s := range snapshots1 {
		parsedSnap, err := uuid.Parse(s.ID)
		if err != nil {
			t.Fatalf("snapshot ID %q is not valid UUID: %v", s.ID, err)
		}
		if parsedSnap.Version() != 7 {
			t.Errorf("snapshot UUID version = %d, want 7", parsedSnap.Version())
		}
		if s.FileID != files1[0].ID {
			t.Errorf("snapshot FileID = %s, want %s", s.FileID, files1[0].ID)
		}
	}

	snapshots2, err := d.GetSnapshots(files2[0].ID)
	if err != nil {
		t.Fatalf("GetSnapshots(file2): %v", err)
	}
	if len(snapshots2) != 1 {
		t.Fatalf("got %d snapshots for file2, want 1", len(snapshots2))
	}
	if snapshots2[0].FileID != files2[0].ID {
		t.Errorf("snapshot FileID = %s, want %s", snapshots2[0].FileID, files2[0].ID)
	}

	// Verify stats are correct
	stats, err := d.GetStats()
	if err != nil {
		t.Fatalf("GetStats(): %v", err)
	}
	if stats.TotalFiles != 2 {
		t.Errorf("TotalFiles = %d, want 2", stats.TotalFiles)
	}
	if stats.TotalSnapshots != 3 {
		t.Errorf("TotalSnapshots = %d, want 3", stats.TotalSnapshots)
	}
}

func TestSaveRename_Basic(t *testing.T) {
	d := newTestDB(t, 0)

	// Create a file with a snapshot
	if _, err := d.SaveSnapshot("/tmp/old.go", []byte("package main")); err != nil {
		t.Fatal(err)
	}

	// Save a rename
	newFileID, err := d.SaveRename("/tmp/old.go", "/tmp/new.go")
	if err != nil {
		t.Fatalf("SaveRename() error: %v", err)
	}
	if newFileID == "" {
		t.Fatal("SaveRename() returned empty newFileID")
	}

	// Verify new file was created
	newFile, err := d.GetFile(newFileID)
	if err != nil {
		t.Fatalf("GetFile(newFileID) error: %v", err)
	}
	if newFile.Path != "/tmp/new.go" {
		t.Errorf("new file path = %s, want /tmp/new.go", newFile.Path)
	}

	// Verify rename record
	oldFiles, err := d.SearchFiles("old.go", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	renames, err := d.GetRenames(oldFiles[0].ID)
	if err != nil {
		t.Fatalf("GetRenames() error: %v", err)
	}
	if len(renames) != 1 {
		t.Fatalf("got %d renames, want 1", len(renames))
	}
	if renames[0].OldPath != "/tmp/old.go" {
		t.Errorf("OldPath = %s, want /tmp/old.go", renames[0].OldPath)
	}
	if renames[0].NewPath != "/tmp/new.go" {
		t.Errorf("NewPath = %s, want /tmp/new.go", renames[0].NewPath)
	}
}

func TestSaveRename_ChainedRenames(t *testing.T) {
	d := newTestDB(t, 0)

	// Create initial file
	if _, err := d.SaveSnapshot("/tmp/a.go", []byte("package main")); err != nil {
		t.Fatal(err)
	}

	// A -> B
	bFileID, err := d.SaveRename("/tmp/a.go", "/tmp/b.go")
	if err != nil {
		t.Fatalf("SaveRename(a->b) error: %v", err)
	}

	// Save snapshot for B so it exists
	if _, err := d.SaveSnapshot("/tmp/b.go", []byte("package main")); err != nil {
		t.Fatal(err)
	}

	// B -> C
	_, err = d.SaveRename("/tmp/b.go", "/tmp/c.go")
	if err != nil {
		t.Fatalf("SaveRename(b->c) error: %v", err)
	}

	// Check renames from B's perspective (should see both A->B and B->C)
	renames, err := d.GetRenames(bFileID)
	if err != nil {
		t.Fatalf("GetRenames(b) error: %v", err)
	}
	if len(renames) != 2 {
		t.Fatalf("got %d renames for B, want 2", len(renames))
	}
	// Ordered by timestamp ASC
	if renames[0].OldPath != "/tmp/a.go" || renames[0].NewPath != "/tmp/b.go" {
		t.Errorf("renames[0] = %s->%s, want a.go->b.go", renames[0].OldPath, renames[0].NewPath)
	}
	if renames[1].OldPath != "/tmp/b.go" || renames[1].NewPath != "/tmp/c.go" {
		t.Errorf("renames[1] = %s->%s, want b.go->c.go", renames[1].OldPath, renames[1].NewPath)
	}
}

func TestSaveRename_OldFileNotFound(t *testing.T) {
	d := newTestDB(t, 0)

	newFileID, err := d.SaveRename("/tmp/nonexistent.go", "/tmp/new.go")
	if err != nil {
		t.Fatalf("SaveRename() unexpected error: %v", err)
	}
	if newFileID != "" {
		t.Errorf("SaveRename() returned %q, want empty string for untracked old file", newFileID)
	}
}

func TestGetRenames_Empty(t *testing.T) {
	d := newTestDB(t, 0)

	if _, err := d.SaveSnapshot("/tmp/norenames.go", []byte("content")); err != nil {
		t.Fatal(err)
	}
	files, err := d.SearchFiles("norenames.go", 10, 0)
	if err != nil {
		t.Fatal(err)
	}

	renames, err := d.GetRenames(files[0].ID)
	if err != nil {
		t.Fatalf("GetRenames() error: %v", err)
	}
	if len(renames) != 0 {
		t.Errorf("got %d renames, want 0", len(renames))
	}
}

func TestSaveRename_ExistingNewFile(t *testing.T) {
	d := newTestDB(t, 0)

	// Create both files
	if _, err := d.SaveSnapshot("/tmp/old2.go", []byte("old")); err != nil {
		t.Fatal(err)
	}
	if _, err := d.SaveSnapshot("/tmp/existing.go", []byte("existing")); err != nil {
		t.Fatal(err)
	}

	// Rename to existing file path
	newFileID, err := d.SaveRename("/tmp/old2.go", "/tmp/existing.go")
	if err != nil {
		t.Fatalf("SaveRename() error: %v", err)
	}

	// Should reuse the existing file ID
	existingFiles, err := d.SearchFiles("existing.go", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if newFileID != existingFiles[0].ID {
		t.Errorf("newFileID = %s, want %s (existing file ID)", newFileID, existingFiles[0].ID)
	}
}

func TestMigrateIfNeeded_AlreadyNewSchema(t *testing.T) {
	// New DB already has TEXT schema; migration should be a no-op
	d := newTestDB(t, 0)

	if _, err := d.SaveSnapshot("/tmp/new.go", []byte("content")); err != nil {
		t.Fatal(err)
	}

	files, err := d.SearchFiles("new.go", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}

	// Verify ID is valid UUIDv7 (not affected by migration)
	parsed, err := uuid.Parse(files[0].ID)
	if err != nil {
		t.Fatalf("ID %q is not valid UUID: %v", files[0].ID, err)
	}
	if parsed.Version() != 7 {
		t.Errorf("UUID version = %d, want 7", parsed.Version())
	}
}

func TestMigrateIfNeeded_EmptyOldSchema(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "empty_old.db")

	// Create old schema DB with no data
	sqlDB, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on")
	if err != nil {
		t.Fatalf("opening DB: %v", err)
	}
	oldSchema := `
	CREATE TABLE files (
		id       INTEGER PRIMARY KEY AUTOINCREMENT,
		path     TEXT NOT NULL UNIQUE,
		created  INTEGER NOT NULL DEFAULT (unixepoch()),
		updated  INTEGER NOT NULL DEFAULT (unixepoch())
	);
	CREATE TABLE snapshots (
		id        INTEGER PRIMARY KEY AUTOINCREMENT,
		file_id   INTEGER NOT NULL REFERENCES files(id) ON DELETE CASCADE,
		content   BLOB NOT NULL,
		size      INTEGER NOT NULL,
		hash      TEXT NOT NULL,
		timestamp INTEGER NOT NULL DEFAULT (unixepoch())
	);
	`
	if _, err := sqlDB.Exec(oldSchema); err != nil {
		t.Fatalf("creating old schema: %v", err)
	}
	sqlDB.Close()

	// Open with New() — migration should succeed with empty tables
	d, err := New(dbPath, 0)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer d.Close()

	// Should be able to use the DB normally after migration
	saved, err := d.SaveSnapshot("/tmp/post_migrate.go", []byte("after migration"))
	if err != nil {
		t.Fatalf("SaveSnapshot() error: %v", err)
	}
	if !saved {
		t.Error("SaveSnapshot() = false, want true")
	}

	files, err := d.SearchFiles("post_migrate", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	parsed, err := uuid.Parse(files[0].ID)
	if err != nil {
		t.Fatalf("ID %q is not valid UUID: %v", files[0].ID, err)
	}
	if parsed.Version() != 7 {
		t.Errorf("UUID version = %d, want 7", parsed.Version())
	}
}

func TestDatabaseSize(t *testing.T) {
	d := newTestDB(t, 0)

	size, err := d.DatabaseSize()
	if err != nil {
		t.Fatalf("DatabaseSize() error: %v", err)
	}
	if size <= 0 {
		t.Errorf("DatabaseSize() = %d, want > 0", size)
	}
}

func TestCreateDatabaseSnapshot(t *testing.T) {
	d := newTestDB(t, 0)

	// Add some data
	if _, err := d.SaveSnapshot("/tmp/snap_test.go", []byte("package main")); err != nil {
		t.Fatal(err)
	}
	if _, err := d.SaveSnapshot("/tmp/snap_test2.go", []byte("package lib")); err != nil {
		t.Fatal(err)
	}

	tmpDir := t.TempDir()
	snapshotPath, err := d.CreateDatabaseSnapshot(tmpDir)
	if err != nil {
		t.Fatalf("CreateDatabaseSnapshot() error: %v", err)
	}
	defer os.Remove(snapshotPath)

	// Verify the snapshot file exists and is a valid SQLite database
	fi, err := os.Stat(snapshotPath)
	if err != nil {
		t.Fatalf("stat snapshot: %v", err)
	}
	if fi.Size() == 0 {
		t.Error("snapshot file is empty")
	}

	// Open the snapshot and verify it contains the expected data
	snapDB, err := sql.Open("sqlite3", snapshotPath)
	if err != nil {
		t.Fatalf("opening snapshot DB: %v", err)
	}
	defer snapDB.Close()

	var fileCount int
	if err := snapDB.QueryRow("SELECT COUNT(*) FROM files").Scan(&fileCount); err != nil {
		t.Fatalf("counting files in snapshot: %v", err)
	}
	if fileCount != 2 {
		t.Errorf("snapshot has %d files, want 2", fileCount)
	}

	var snapCount int
	if err := snapDB.QueryRow("SELECT COUNT(*) FROM snapshots").Scan(&snapCount); err != nil {
		t.Fatalf("counting snapshots in snapshot: %v", err)
	}
	if snapCount != 2 {
		t.Errorf("snapshot has %d snapshots, want 2", snapCount)
	}
}

func TestCreateDatabaseSnapshot_EmptyDB(t *testing.T) {
	d := newTestDB(t, 0)

	tmpDir := t.TempDir()
	snapshotPath, err := d.CreateDatabaseSnapshot(tmpDir)
	if err != nil {
		t.Fatalf("CreateDatabaseSnapshot() error: %v", err)
	}
	defer os.Remove(snapshotPath)

	fi, err := os.Stat(snapshotPath)
	if err != nil {
		t.Fatalf("stat snapshot: %v", err)
	}
	if fi.Size() == 0 {
		t.Error("snapshot file is empty even for empty DB")
	}
}

func TestMigrateIfNeeded_PostMigrationOperations(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "migrate_ops.db")
	createOldSchemaDB(t, dbPath)

	d, err := New(dbPath, 0)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer d.Close()

	// Save a new snapshot after migration
	saved, err := d.SaveSnapshot("/tmp/old1.go", []byte("updated content"))
	if err != nil {
		t.Fatalf("SaveSnapshot() error: %v", err)
	}
	if !saved {
		t.Error("SaveSnapshot() = false, want true")
	}

	// Verify the new snapshot was added to the existing migrated file
	files, err := d.SearchFiles("old1.go", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	snapshots, err := d.GetSnapshots(files[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	// 2 original + 1 new
	if len(snapshots) != 3 {
		t.Errorf("got %d snapshots, want 3", len(snapshots))
	}

	// Verify GetRecentSnapshots works across migrated and new data
	entries, err := d.GetRecentSnapshots(50)
	if err != nil {
		t.Fatal(err)
	}
	// 3 original + 1 new = 4
	if len(entries) != 4 {
		t.Errorf("got %d recent entries, want 4", len(entries))
	}

	// Verify DeleteFile works on migrated file
	files2, err := d.SearchFiles("old2.go", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.DeleteFile(files2[0].ID); err != nil {
		t.Fatalf("DeleteFile() error: %v", err)
	}

	stats, err := d.GetStats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalFiles != 1 {
		t.Errorf("TotalFiles = %d, want 1", stats.TotalFiles)
	}
}

func TestSaveSnapshotBatch_Basic(t *testing.T) {
	d := newTestDB(t, 0)

	filePaths := []string{"/tmp/a.go", "/tmp/b.go", "/tmp/c.go"}
	contents := [][]byte{[]byte("aaa"), []byte("bbb"), []byte("ccc")}

	saved, errs := d.SaveSnapshotBatch(filePaths, contents)

	for i, err := range errs {
		if err != nil {
			t.Errorf("SaveSnapshotBatch() item %d error: %v", i, err)
		}
	}
	for i, s := range saved {
		if !s {
			t.Errorf("SaveSnapshotBatch() item %d saved = false, want true", i)
		}
	}

	stats, err := d.GetStats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalFiles != 3 {
		t.Errorf("TotalFiles = %d, want 3", stats.TotalFiles)
	}
	if stats.TotalSnapshots != 3 {
		t.Errorf("TotalSnapshots = %d, want 3", stats.TotalSnapshots)
	}
}

func TestSaveSnapshotBatch_DuplicateSkip(t *testing.T) {
	d := newTestDB(t, 0)

	// First batch
	filePaths := []string{"/tmp/dup.go"}
	contents := [][]byte{[]byte("content")}
	d.SaveSnapshotBatch(filePaths, contents)

	// Second batch with same content
	saved, errs := d.SaveSnapshotBatch(filePaths, contents)

	if errs[0] != nil {
		t.Fatalf("SaveSnapshotBatch() error: %v", errs[0])
	}
	if saved[0] {
		t.Error("SaveSnapshotBatch() saved duplicate, want skip")
	}

	stats, err := d.GetStats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalSnapshots != 1 {
		t.Errorf("TotalSnapshots = %d, want 1", stats.TotalSnapshots)
	}
}

func TestSaveSnapshotBatch_ManyFiles(t *testing.T) {
	d := newTestDB(t, 0)

	n := 100
	filePaths := make([]string, n)
	contents := make([][]byte, n)
	for i := range n {
		filePaths[i] = fmt.Sprintf("/tmp/batch%d.go", i)
		contents[i] = []byte(fmt.Sprintf("content %d", i))
	}

	saved, errs := d.SaveSnapshotBatch(filePaths, contents)

	for i, err := range errs {
		if err != nil {
			t.Errorf("item %d error: %v", i, err)
		}
	}
	savedCount := 0
	for _, s := range saved {
		if s {
			savedCount++
		}
	}
	if savedCount != n {
		t.Errorf("saved %d, want %d", savedCount, n)
	}

	stats, err := d.GetStats()
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalFiles != n {
		t.Errorf("TotalFiles = %d, want %d", stats.TotalFiles, n)
	}
}
