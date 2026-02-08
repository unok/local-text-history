package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unok/local-text-history/internal/config"
	"github.com/unok/local-text-history/internal/db"
)

func newTestServer(t *testing.T) (*Server, *db.DB) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.New(dbPath)
	if err != nil {
		t.Fatalf("db.New() error: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	srv := New(database, nil, nil, nil)
	return srv, database
}

func TestSearchFiles_Empty(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/files?q=test", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var files []db.File
	if err := json.NewDecoder(w.Body).Decode(&files); err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Errorf("got %d files, want 0", len(files))
	}
}

func TestSearchFiles_WithResults(t *testing.T) {
	srv, database := newTestServer(t)

	if _, err := database.SaveSnapshot("/tmp/test.go", []byte("package main"), 0); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/api/files?q=test", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var files []db.File
	if err := json.NewDecoder(w.Body).Decode(&files); err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	if files[0].Path != "/tmp/test.go" {
		t.Errorf("path = %s, want /tmp/test.go", files[0].Path)
	}
}

func TestGetFile(t *testing.T) {
	srv, database := newTestServer(t)

	if _, err := database.SaveSnapshot("/tmp/get.go", []byte("content"), 0); err != nil {
		t.Fatal(err)
	}
	files, _ := database.SearchFiles("get.go", 1, 0, nil)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/files/%s", files[0].ID), nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var file db.File
	if err := json.NewDecoder(w.Body).Decode(&file); err != nil {
		t.Fatal(err)
	}
	if file.Path != "/tmp/get.go" {
		t.Errorf("path = %s, want /tmp/get.go", file.Path)
	}
}

func TestGetFile_NotFound(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/files/00000000-0000-7000-8000-000000000000", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestGetFile_InvalidID(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/files/abc", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestGetSnapshots(t *testing.T) {
	srv, database := newTestServer(t)

	if _, err := database.SaveSnapshot("/tmp/snap.go", []byte("v1"), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := database.SaveSnapshot("/tmp/snap.go", []byte("v2"), 0); err != nil {
		t.Fatal(err)
	}
	files, _ := database.SearchFiles("snap.go", 1, 0, nil)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/files/%s/snapshots", files[0].ID), nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var snapshots []db.Snapshot
	if err := json.NewDecoder(w.Body).Decode(&snapshots); err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != 2 {
		t.Errorf("got %d snapshots, want 2", len(snapshots))
	}
}

func TestGetSnapshot_WithContent(t *testing.T) {
	srv, database := newTestServer(t)

	if _, err := database.SaveSnapshot("/tmp/content.go", []byte("package main"), 0); err != nil {
		t.Fatal(err)
	}
	files, _ := database.SearchFiles("content.go", 1, 0, nil)
	snapshots, _ := database.GetSnapshots(files[0].ID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/snapshots/%s", snapshots[0].ID), nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var result struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Content != "package main" {
		t.Errorf("content = %q, want %q", result.Content, "package main")
	}
}

func TestGetSnapshot_NotFound(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/snapshots/00000000-0000-7000-8000-000000000000", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestDownloadSnapshot(t *testing.T) {
	srv, database := newTestServer(t)

	if _, err := database.SaveSnapshot("/tmp/download.go", []byte("package main"), 0); err != nil {
		t.Fatal(err)
	}
	files, _ := database.SearchFiles("download.go", 1, 0, nil)
	snapshots, _ := database.GetSnapshots(files[0].ID)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/snapshots/%s/download", snapshots[0].ID), nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/octet-stream" {
		t.Errorf("content-type = %s, want application/octet-stream", ct)
	}
	if cd := w.Header().Get("Content-Disposition"); cd == "" {
		t.Error("missing Content-Disposition header")
	}
	if w.Body.String() != "package main" {
		t.Errorf("body = %q, want %q", w.Body.String(), "package main")
	}
}

func TestDiff(t *testing.T) {
	srv, database := newTestServer(t)

	if _, err := database.SaveSnapshot("/tmp/diff.go", []byte("line1\nline2\n"), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := database.SaveSnapshot("/tmp/diff.go", []byte("line1\nmodified\n"), 0); err != nil {
		t.Fatal(err)
	}
	files, _ := database.SearchFiles("diff.go", 1, 0, nil)
	snapshots, _ := database.GetSnapshots(files[0].ID)

	// snapshots are newest first
	fromID := snapshots[1].ID
	toID := snapshots[0].ID

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/diff?from=%s&to=%s", fromID, toID), nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var result struct {
		Diff string `json:"diff"`
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Diff == "" {
		t.Error("diff should not be empty")
	}
}

func TestDiff_MissingTo(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/diff", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestDiff_InitialSnapshot(t *testing.T) {
	srv, database := newTestServer(t)

	if _, err := database.SaveSnapshot("/tmp/initial.go", []byte("package main\n"), 0); err != nil {
		t.Fatal(err)
	}
	files, _ := database.SearchFiles("initial.go", 1, 0, nil)
	snapshots, _ := database.GetSnapshots(files[0].ID)

	// Only 'to' parameter, no 'from' — should compare against empty content
	req := httptest.NewRequest("GET", fmt.Sprintf("/api/diff?to=%s", snapshots[0].ID), nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var result struct {
		Diff string `json:"diff"`
		From string `json:"from"`
		To   string `json:"to"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Diff == "" {
		t.Error("diff should not be empty for initial snapshot")
	}
	if result.From != "" {
		t.Errorf("from = %q, want empty string", result.From)
	}
	if result.To != snapshots[0].ID {
		t.Errorf("to = %s, want %s", result.To, snapshots[0].ID)
	}
	if !strings.Contains(result.Diff, "+package main") {
		t.Errorf("diff should show content as additions, got: %s", result.Diff)
	}
}

func TestStats(t *testing.T) {
	srv, database := newTestServer(t)

	if _, err := database.SaveSnapshot("/tmp/stats.go", []byte("content"), 0); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/api/stats", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var result struct {
		TotalFiles     int            `json:"totalFiles"`
		TotalSnapshots int            `json:"totalSnapshots"`
		WatchSets      []watchSetInfo `json:"watchSets"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.TotalFiles != 1 {
		t.Errorf("TotalFiles = %d, want 1", result.TotalFiles)
	}
	if result.TotalSnapshots != 1 {
		t.Errorf("TotalSnapshots = %d, want 1", result.TotalSnapshots)
	}
}

func TestStats_IncludesWatchSets(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.New(dbPath)
	if err != nil {
		t.Fatalf("db.New() error: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	watchSets := []config.WatchSet{
		{Name: "Projects", Dirs: []string{"/home/user/projects"}},
		{Name: "Docs", Dirs: []string{"/home/user/docs"}},
	}
	srv := New(database, nil, watchSets, nil)

	req := httptest.NewRequest("GET", "/api/stats", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var result struct {
		TotalFiles     int            `json:"totalFiles"`
		TotalSnapshots int            `json:"totalSnapshots"`
		TotalSize      int64          `json:"totalSize"`
		WatchDirs      []string       `json:"watchDirs"`
		WatchSets      []watchSetInfo `json:"watchSets"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	// Check watchDirs (backward compatible)
	if len(result.WatchDirs) != 2 {
		t.Fatalf("got %d watchDirs, want 2", len(result.WatchDirs))
	}
	if result.WatchDirs[0] != "/home/user/projects" {
		t.Errorf("watchDirs[0] = %s, want /home/user/projects", result.WatchDirs[0])
	}
	if result.WatchDirs[1] != "/home/user/docs" {
		t.Errorf("watchDirs[1] = %s, want /home/user/docs", result.WatchDirs[1])
	}
	// Check watchSets
	if len(result.WatchSets) != 2 {
		t.Fatalf("got %d watchSets, want 2", len(result.WatchSets))
	}
	if result.WatchSets[0].Name != "Projects" {
		t.Errorf("watchSets[0].Name = %s, want Projects", result.WatchSets[0].Name)
	}
	if result.WatchSets[1].Name != "Docs" {
		t.Errorf("watchSets[1].Name = %s, want Docs", result.WatchSets[1].Name)
	}
}

func TestDeleteFile(t *testing.T) {
	srv, database := newTestServer(t)

	if _, err := database.SaveSnapshot("/tmp/delete.go", []byte("content"), 0); err != nil {
		t.Fatal(err)
	}
	files, _ := database.SearchFiles("delete.go", 1, 0, nil)

	req := httptest.NewRequest("DELETE", fmt.Sprintf("/api/files/%s", files[0].ID), nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNoContent)
	}

	// Verify deletion
	_, err := database.GetFile(files[0].ID)
	if err == nil {
		t.Error("file should be deleted")
	}
}

func TestDeleteFile_NotFound(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest("DELETE", "/api/files/00000000-0000-7000-8000-000000000000", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestSPA_APINotFound(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/nonexistent", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestSearchFiles_Pagination(t *testing.T) {
	srv, database := newTestServer(t)

	for i := range 5 {
		path := fmt.Sprintf("/tmp/page%d.go", i)
		if _, err := database.SaveSnapshot(path, []byte("content"), 0); err != nil {
			t.Fatal(err)
		}
	}

	req := httptest.NewRequest("GET", "/api/files?q=page&limit=2&offset=0", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	var files []db.File
	if err := json.NewDecoder(w.Body).Decode(&files); err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Errorf("got %d files, want 2", len(files))
	}
}

func TestHandleHistory_Empty(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/history", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var result struct {
		Entries []db.HistoryEntry `json:"entries"`
		HasMore bool             `json:"hasMore"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 0 {
		t.Errorf("got %d entries, want 0", len(result.Entries))
	}
	if result.HasMore {
		t.Error("hasMore = true, want false")
	}
}

func TestHandleHistory_WithData(t *testing.T) {
	srv, database := newTestServer(t)

	if _, err := database.SaveSnapshot("/tmp/hist1.go", []byte("content1"), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := database.SaveSnapshot("/tmp/hist2.go", []byte("content2"), 0); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/api/history", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var result struct {
		Entries []db.HistoryEntry `json:"entries"`
		HasMore bool             `json:"hasMore"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 2 {
		t.Errorf("got %d entries, want 2", len(result.Entries))
	}
	if result.HasMore {
		t.Error("hasMore = true, want false")
	}

	// Verify newest first
	if result.Entries[0].FilePath != "/tmp/hist2.go" {
		t.Errorf("entries[0].FilePath = %s, want /tmp/hist2.go", result.Entries[0].FilePath)
	}
	if result.Entries[1].FilePath != "/tmp/hist1.go" {
		t.Errorf("entries[1].FilePath = %s, want /tmp/hist1.go", result.Entries[1].FilePath)
	}
}

func TestHandleHistory_CustomLimit(t *testing.T) {
	srv, database := newTestServer(t)

	for i := range 5 {
		path := fmt.Sprintf("/tmp/hlimit%d.go", i)
		if _, err := database.SaveSnapshot(path, []byte(fmt.Sprintf("content%d", i)), 0); err != nil {
			t.Fatal(err)
		}
	}

	req := httptest.NewRequest("GET", "/api/history?limit=3", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var result struct {
		Entries []db.HistoryEntry `json:"entries"`
		HasMore bool             `json:"hasMore"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 3 {
		t.Errorf("got %d entries, want 3", len(result.Entries))
	}
	if !result.HasMore {
		t.Error("hasMore = false, want true (5 items with limit=3)")
	}
}

func TestHandleHistory_Pagination(t *testing.T) {
	srv, database := newTestServer(t)

	for i := range 5 {
		path := fmt.Sprintf("/tmp/hpage%d.go", i)
		if _, err := database.SaveSnapshot(path, []byte(fmt.Sprintf("content%d", i)), 0); err != nil {
			t.Fatal(err)
		}
	}

	// Page 1: offset=0, limit=2
	req := httptest.NewRequest("GET", "/api/history?limit=2&offset=0", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	var page1 struct {
		Entries []db.HistoryEntry `json:"entries"`
		HasMore bool             `json:"hasMore"`
	}
	if err := json.NewDecoder(w.Body).Decode(&page1); err != nil {
		t.Fatal(err)
	}
	if len(page1.Entries) != 2 {
		t.Errorf("page1: got %d entries, want 2", len(page1.Entries))
	}
	if !page1.HasMore {
		t.Error("page1: hasMore = false, want true")
	}

	// Page 3: offset=4, limit=2
	req = httptest.NewRequest("GET", "/api/history?limit=2&offset=4", nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	var page3 struct {
		Entries []db.HistoryEntry `json:"entries"`
		HasMore bool             `json:"hasMore"`
	}
	if err := json.NewDecoder(w.Body).Decode(&page3); err != nil {
		t.Fatal(err)
	}
	if len(page3.Entries) != 1 {
		t.Errorf("page3: got %d entries, want 1", len(page3.Entries))
	}
	if page3.HasMore {
		t.Error("page3: hasMore = true, want false")
	}
}

func TestHandleHistory_IncludesRenames(t *testing.T) {
	srv, database := newTestServer(t)

	if _, err := database.SaveSnapshot("/tmp/hren1.go", []byte("content"), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := database.SaveRename("/tmp/hren1.go", "/tmp/hren2.go"); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/api/history", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var result struct {
		Entries []db.HistoryEntry `json:"entries"`
		HasMore bool             `json:"hasMore"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 2 {
		t.Fatalf("got %d entries, want 2 (1 save + 1 rename)", len(result.Entries))
	}

	// Most recent first: rename, then save
	if result.Entries[0].EntryType != "rename" {
		t.Errorf("entries[0].EntryType = %s, want rename", result.Entries[0].EntryType)
	}
	if result.Entries[0].FilePath != "/tmp/hren2.go" {
		t.Errorf("entries[0].FilePath = %s, want /tmp/hren2.go", result.Entries[0].FilePath)
	}
	if result.Entries[0].OldFilePath != "/tmp/hren1.go" {
		t.Errorf("entries[0].OldFilePath = %s, want /tmp/hren1.go", result.Entries[0].OldFilePath)
	}
	if result.Entries[1].EntryType != "save" {
		t.Errorf("entries[1].EntryType = %s, want save", result.Entries[1].EntryType)
	}
}

func TestGetRenames_Empty(t *testing.T) {
	srv, database := newTestServer(t)

	if _, err := database.SaveSnapshot("/tmp/norename.go", []byte("content"), 0); err != nil {
		t.Fatal(err)
	}
	files, _ := database.SearchFiles("norename.go", 1, 0, nil)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/files/%s/renames", files[0].ID), nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var renames []db.Rename
	if err := json.NewDecoder(w.Body).Decode(&renames); err != nil {
		t.Fatal(err)
	}
	if len(renames) != 0 {
		t.Errorf("got %d renames, want 0", len(renames))
	}
}

func TestGetRenames_WithData(t *testing.T) {
	srv, database := newTestServer(t)

	if _, err := database.SaveSnapshot("/tmp/renold.go", []byte("content"), 0); err != nil {
		t.Fatal(err)
	}

	_, err := database.SaveRename("/tmp/renold.go", "/tmp/rennew.go")
	if err != nil {
		t.Fatalf("SaveRename() error: %v", err)
	}

	files, _ := database.SearchFiles("renold.go", 1, 0, nil)

	req := httptest.NewRequest("GET", fmt.Sprintf("/api/files/%s/renames", files[0].ID), nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var renames []db.Rename
	if err := json.NewDecoder(w.Body).Decode(&renames); err != nil {
		t.Fatal(err)
	}
	if len(renames) != 1 {
		t.Fatalf("got %d renames, want 1", len(renames))
	}
	if renames[0].OldPath != "/tmp/renold.go" {
		t.Errorf("OldPath = %s, want /tmp/renold.go", renames[0].OldPath)
	}
	if renames[0].NewPath != "/tmp/rennew.go" {
		t.Errorf("NewPath = %s, want /tmp/rennew.go", renames[0].NewPath)
	}
}

func TestGetRenames_InvalidID(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/files/abc/renames", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestDatabaseDownload(t *testing.T) {
	srv, database := newTestServer(t)

	if _, err := database.SaveSnapshot("/tmp/dbdl.go", []byte("package main"), 0); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/api/database/download", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/x-sqlite3") {
		t.Errorf("content-type = %s, want application/x-sqlite3", ct)
	}
	if cd := w.Header().Get("Content-Disposition"); cd == "" {
		t.Error("missing Content-Disposition header")
	} else if !strings.Contains(cd, "history-") || !strings.Contains(cd, ".db") {
		t.Errorf("Content-Disposition = %s, want to contain history-*.db", cd)
	}
	if w.Body.Len() == 0 {
		t.Error("response body is empty")
	}

	// Verify the downloaded content is a valid SQLite database
	// SQLite magic bytes: "SQLite format 3\000"
	body := w.Body.Bytes()
	if len(body) < 16 {
		t.Fatal("response body too short for SQLite header")
	}
	magic := string(body[:16])
	if magic != "SQLite format 3\000" {
		t.Errorf("not a valid SQLite file, magic = %q", magic)
	}
}

func TestDatabaseDownload_EmptyDB(t *testing.T) {
	srv, _ := newTestServer(t)

	req := httptest.NewRequest("GET", "/api/database/download", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if w.Body.Len() == 0 {
		t.Error("response body is empty even for empty database")
	}
}

func TestHandleSSE_Connection(t *testing.T) {
	srv, _ := newTestServer(t)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/events", nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %s, want text/event-stream", ct)
	}
	if cc := resp.Header.Get("Cache-Control"); cc != "no-cache" {
		t.Errorf("Cache-Control = %s, want no-cache", cc)
	}
}

func TestHandleSSE_ReceivesNotification(t *testing.T) {
	srv, _ := newTestServer(t)

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/events", nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	// Wait briefly for the SSE client to register
	time.Sleep(100 * time.Millisecond)

	// Send a notification
	srv.Notify("/tmp/notified.go")

	// Read the SSE data line
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if !strings.Contains(data, "/tmp/notified.go") {
				t.Errorf("SSE data = %s, want to contain /tmp/notified.go", data)
			}
			return
		}
	}
	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		t.Fatalf("scanner error: %v", err)
	}
	if ctx.Err() != nil {
		t.Fatal("timed out waiting for SSE event")
	}
}

func TestBasicAuth_RejectsWithoutCredentials(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.New(dbPath)
	if err != nil {
		t.Fatalf("db.New() error: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	auth := &config.BasicAuthConfig{Username: "admin", Password: "secret"}
	srv := New(database, nil, nil, auth)

	req := httptest.NewRequest("GET", "/api/stats", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
	if hdr := w.Header().Get("WWW-Authenticate"); hdr == "" {
		t.Error("missing WWW-Authenticate header")
	}
}

func TestBasicAuth_RejectsWrongCredentials(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.New(dbPath)
	if err != nil {
		t.Fatalf("db.New() error: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	auth := &config.BasicAuthConfig{Username: "admin", Password: "secret"}
	srv := New(database, nil, nil, auth)

	req := httptest.NewRequest("GET", "/api/stats", nil)
	req.SetBasicAuth("admin", "wrong")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestBasicAuth_AcceptsValidCredentials(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.New(dbPath)
	if err != nil {
		t.Fatalf("db.New() error: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	auth := &config.BasicAuthConfig{Username: "admin", Password: "secret"}
	srv := New(database, nil, nil, auth)

	req := httptest.NewRequest("GET", "/api/stats", nil)
	req.SetBasicAuth("admin", "secret")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleHistory_QueryFilter(t *testing.T) {
	srv, database := newTestServer(t)

	if _, err := database.SaveSnapshot("/tmp/project/src/main.go", []byte("package main"), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := database.SaveSnapshot("/tmp/project/src/util.go", []byte("package util"), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := database.SaveSnapshot("/tmp/project/test/main_test.go", []byte("package test"), 0); err != nil {
		t.Fatal(err)
	}

	// Given: query matching "main"
	req := httptest.NewRequest("GET", "/api/history?q=main", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var result struct {
		Entries []db.HistoryEntry `json:"entries"`
		HasMore bool             `json:"hasMore"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 2 {
		t.Errorf("got %d entries, want 2", len(result.Entries))
	}
}

func TestHandleHistory_QueryFilterWithPagination(t *testing.T) {
	srv, database := newTestServer(t)

	for i := range 5 {
		path := fmt.Sprintf("/tmp/srv_qp%d.go", i)
		if _, err := database.SaveSnapshot(path, []byte(fmt.Sprintf("content%d", i)), 0); err != nil {
			t.Fatal(err)
		}
	}
	// Non-matching entry
	if _, err := database.SaveSnapshot("/tmp/other.go", []byte("other"), 0); err != nil {
		t.Fatal(err)
	}

	// Page 1: limit=3, query=srv_qp
	req := httptest.NewRequest("GET", "/api/history?q=srv_qp&limit=3&offset=0", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("page1: status = %d, want %d", w.Code, http.StatusOK)
	}

	var page1 struct {
		Entries []db.HistoryEntry `json:"entries"`
		HasMore bool             `json:"hasMore"`
	}
	if err := json.NewDecoder(w.Body).Decode(&page1); err != nil {
		t.Fatal(err)
	}
	if len(page1.Entries) != 3 {
		t.Errorf("page1: got %d entries, want 3", len(page1.Entries))
	}
	if !page1.HasMore {
		t.Error("page1: hasMore = false, want true")
	}

	// Page 2: limit=3, offset=3, query=srv_qp
	req = httptest.NewRequest("GET", "/api/history?q=srv_qp&limit=3&offset=3", nil)
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("page2: status = %d, want %d", w.Code, http.StatusOK)
	}

	var page2 struct {
		Entries []db.HistoryEntry `json:"entries"`
		HasMore bool             `json:"hasMore"`
	}
	if err := json.NewDecoder(w.Body).Decode(&page2); err != nil {
		t.Fatal(err)
	}
	if len(page2.Entries) != 2 {
		t.Errorf("page2: got %d entries, want 2", len(page2.Entries))
	}
	if page2.HasMore {
		t.Error("page2: hasMore = true, want false")
	}
}

func TestHandleHistory_EmptyQueryReturnsAll(t *testing.T) {
	srv, database := newTestServer(t)

	if _, err := database.SaveSnapshot("/tmp/eqr1.go", []byte("c1"), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := database.SaveSnapshot("/tmp/eqr2.go", []byte("c2"), 0); err != nil {
		t.Fatal(err)
	}

	// Given: empty q parameter
	req := httptest.NewRequest("GET", "/api/history?q=", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var result struct {
		Entries []db.HistoryEntry `json:"entries"`
		HasMore bool             `json:"hasMore"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 2 {
		t.Errorf("got %d entries, want 2", len(result.Entries))
	}
}

func TestBasicAuth_NilConfigSkipsAuth(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.New(dbPath)
	if err != nil {
		t.Fatalf("db.New() error: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	srv := New(database, nil, nil, nil)

	req := httptest.NewRequest("GET", "/api/stats", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

// Tests for WatchSet filtering in API

func TestHandleHistory_WatchSetFilter(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.New(dbPath)
	if err != nil {
		t.Fatalf("db.New() error: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	watchSets := []config.WatchSet{
		{Name: "project-a", Dirs: []string{"/home/user/project-a"}},
		{Name: "project-b", Dirs: []string{"/home/user/project-b"}},
	}
	srv := New(database, nil, watchSets, nil)

	// Save snapshots in different dirs
	if _, err := database.SaveSnapshot("/home/user/project-a/main.go", []byte("a"), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := database.SaveSnapshot("/home/user/project-b/main.go", []byte("b"), 0); err != nil {
		t.Fatal(err)
	}

	// Filter by project-a
	req := httptest.NewRequest("GET", "/api/history?watchSet=project-a", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var result struct {
		Entries []db.HistoryEntry `json:"entries"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(result.Entries))
	}
	if result.Entries[0].FilePath != "/home/user/project-a/main.go" {
		t.Errorf("filePath = %s, want /home/user/project-a/main.go", result.Entries[0].FilePath)
	}
}

func TestHandleHistory_UnknownWatchSetReturnsAll(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.New(dbPath)
	if err != nil {
		t.Fatalf("db.New() error: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	watchSets := []config.WatchSet{
		{Name: "project-a", Dirs: []string{"/home/user/project-a"}},
	}
	srv := New(database, nil, watchSets, nil)

	if _, err := database.SaveSnapshot("/home/user/project-a/main.go", []byte("a"), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := database.SaveSnapshot("/home/user/other/main.go", []byte("b"), 0); err != nil {
		t.Fatal(err)
	}

	// Unknown watchSet name should return all entries (no filter)
	req := httptest.NewRequest("GET", "/api/history?watchSet=unknown", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	var result struct {
		Entries []db.HistoryEntry `json:"entries"`
	}
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 2 {
		t.Errorf("got %d entries, want 2 (unknown watchSet = no filter)", len(result.Entries))
	}
}

func TestSearchFiles_WatchSetFilter(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := db.New(dbPath)
	if err != nil {
		t.Fatalf("db.New() error: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	watchSets := []config.WatchSet{
		{Name: "project-a", Dirs: []string{"/home/user/project-a"}},
		{Name: "project-b", Dirs: []string{"/home/user/project-b"}},
	}
	srv := New(database, nil, watchSets, nil)

	if _, err := database.SaveSnapshot("/home/user/project-a/main.go", []byte("a"), 0); err != nil {
		t.Fatal(err)
	}
	if _, err := database.SaveSnapshot("/home/user/project-b/app.go", []byte("b"), 0); err != nil {
		t.Fatal(err)
	}

	// Filter by project-b
	req := httptest.NewRequest("GET", "/api/files?q=&watchSet=project-b", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var files []db.File
	if err := json.NewDecoder(w.Body).Decode(&files); err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("got %d files, want 1", len(files))
	}
	if files[0].Path != "/home/user/project-b/app.go" {
		t.Errorf("path = %s, want /home/user/project-b/app.go", files[0].Path)
	}
}

func TestResolveDirPrefixes(t *testing.T) {
	watchSets := []config.WatchSet{
		{Name: "proj-a", Dirs: []string{"/a", "/b"}},
		{Name: "proj-b", Dirs: []string{"/c"}},
	}
	srv := New(nil, nil, watchSets, nil)

	// Empty name returns nil
	if got := srv.resolveDirPrefixes(""); got != nil {
		t.Errorf("resolveDirPrefixes(\"\") = %v, want nil", got)
	}

	// Known name returns dirs
	got := srv.resolveDirPrefixes("proj-a")
	if len(got) != 2 || got[0] != "/a" || got[1] != "/b" {
		t.Errorf("resolveDirPrefixes(\"proj-a\") = %v, want [/a /b]", got)
	}

	// Unknown name returns nil
	if got := srv.resolveDirPrefixes("unknown"); got != nil {
		t.Errorf("resolveDirPrefixes(\"unknown\") = %v, want nil", got)
	}
}
