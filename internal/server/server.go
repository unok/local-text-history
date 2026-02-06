package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/unok/local-text-history/internal/db"
	"github.com/unok/local-text-history/internal/diff"
)

// Server handles HTTP requests for the file history API.
type Server struct {
	db         *db.DB
	staticFS   fs.FS
	watchDirs  []string
	mux        *http.ServeMux
	sseClients map[chan string]struct{}
	sseMu      sync.Mutex
}

// New creates a new Server with the given database, static file system, and watch directories.
func New(database *db.DB, staticFS fs.FS, watchDirs []string) *Server {
	s := &Server{
		db:         database,
		staticFS:   staticFS,
		watchDirs:  watchDirs,
		mux:        http.NewServeMux(),
		sseClients: make(map[chan string]struct{}),
	}
	s.registerRoutes()
	return s
}

// sseEvent represents an SSE notification payload.
type sseEvent struct {
	Type      string `json:"type"`
	FilePath  string `json:"filePath"`
	Timestamp int64  `json:"timestamp"`
}

// Notify sends an SSE event to all connected clients.
func (s *Server) Notify(filePath string) {
	data, err := json.Marshal(sseEvent{
		Type:      "snapshot",
		FilePath:  filePath,
		Timestamp: time.Now().Unix(),
	})
	if err != nil {
		log.Printf("error marshaling SSE event: %v", err)
		return
	}
	event := string(data)

	s.sseMu.Lock()
	defer s.sseMu.Unlock()

	for ch := range s.sseClients {
		// Non-blocking send: skip slow clients
		select {
		case ch <- event:
		default:
		}
	}
}

// Handler returns the HTTP handler for this server.
func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("GET /api/history", s.handleHistory)
	s.mux.HandleFunc("GET /api/events", s.handleSSE)
	s.mux.HandleFunc("GET /api/files", s.handleSearchFiles)
	s.mux.HandleFunc("GET /api/files/{id}", s.handleGetFile)
	s.mux.HandleFunc("GET /api/files/{id}/snapshots", s.handleGetSnapshots)
	s.mux.HandleFunc("GET /api/snapshots/{id}", s.handleGetSnapshot)
	s.mux.HandleFunc("GET /api/snapshots/{id}/download", s.handleDownloadSnapshot)
	s.mux.HandleFunc("GET /api/diff", s.handleDiff)
	s.mux.HandleFunc("GET /api/stats", s.handleStats)
	s.mux.HandleFunc("GET /api/database/download", s.handleDatabaseDownload)
	s.mux.HandleFunc("DELETE /api/files/{id}", s.handleDeleteFile)
	s.mux.HandleFunc("/", s.handleSPA)
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	entries, err := s.db.GetRecentSnapshots(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if entries == nil {
		entries = []db.HistoryEntry{}
	}
	writeJSON(w, http.StatusOK, entries)
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("streaming not supported"))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ch := make(chan string, 16)
	s.sseMu.Lock()
	s.sseClients[ch] = struct{}{}
	s.sseMu.Unlock()

	defer func() {
		s.sseMu.Lock()
		delete(s.sseClients, ch)
		s.sseMu.Unlock()
	}()

	for {
		select {
		case <-r.Context().Done():
			return
		case event := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", event)
			flusher.Flush()
		}
	}
}

func (s *Server) handleSearchFiles(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	files, err := s.db.SearchFiles(query, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if files == nil {
		files = []db.File{}
	}
	writeJSON(w, http.StatusOK, files)
}

func (s *Server) handleGetFile(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	file, err := s.db.GetFile(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, fmt.Errorf("file not found"))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, file)
}

func (s *Server) handleGetSnapshots(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	snapshots, err := s.db.GetSnapshots(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if snapshots == nil {
		snapshots = []db.Snapshot{}
	}
	writeJSON(w, http.StatusOK, snapshots)
}

func (s *Server) handleGetSnapshot(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	snapshot, err := s.db.GetSnapshot(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, fmt.Errorf("snapshot not found"))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	type snapshotResponse struct {
		ID        string `json:"id"`
		FileID    string `json:"fileId"`
		Content   string `json:"content"`
		Size      int64  `json:"size"`
		Hash      string `json:"hash"`
		Timestamp int64  `json:"timestamp"`
	}
	writeJSON(w, http.StatusOK, snapshotResponse{
		ID:        snapshot.ID,
		FileID:    snapshot.FileID,
		Content:   string(snapshot.Content),
		Size:      snapshot.Size,
		Hash:      snapshot.Hash,
		Timestamp: snapshot.Timestamp,
	})
}

func (s *Server) handleDownloadSnapshot(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	snapshot, err := s.db.GetSnapshot(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, fmt.Errorf("snapshot not found"))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	// Get the file to use its path for the filename
	file, err := s.db.GetFile(snapshot.FileID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	filename := filepath.Base(file.Path)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(snapshot.Size, 10))
	w.Write(snapshot.Content)
}

func (s *Server) handleDiff(w http.ResponseWriter, r *http.Request) {
	toID, err := parseUUIDParam(r.URL.Query().Get("to"), "to")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	toSnap, err := s.db.GetSnapshot(toID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, fmt.Errorf("'to' snapshot not found"))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	file, err := s.db.GetFile(toSnap.FileID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	label := file.Path

	// 'from' is optional: when omitted, compare against empty content (initial snapshot)
	var fromContent string
	var fromID string
	fromParam := r.URL.Query().Get("from")
	if fromParam != "" {
		var parseErr error
		fromID, parseErr = parseUUIDParam(fromParam, "from")
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, parseErr)
			return
		}

		fromSnap, snapErr := s.db.GetSnapshot(fromID)
		if snapErr != nil {
			if errors.Is(snapErr, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, fmt.Errorf("'from' snapshot not found"))
				return
			}
			writeError(w, http.StatusInternalServerError, snapErr)
			return
		}
		fromContent = string(fromSnap.Content)
	}

	unifiedDiff := diff.UnifiedDiff(fromContent, string(toSnap.Content), label, label)

	type diffResponse struct {
		Diff string `json:"diff"`
		From string `json:"from"`
		To   string `json:"to"`
	}
	writeJSON(w, http.StatusOK, diffResponse{
		Diff: unifiedDiff,
		From: fromID,
		To:   toID,
	})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.db.GetStats()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	type statsResponse struct {
		TotalFiles     int      `json:"totalFiles"`
		TotalSnapshots int      `json:"totalSnapshots"`
		TotalSize      int64    `json:"totalSize"`
		WatchDirs      []string `json:"watchDirs"`
	}
	dirs := s.watchDirs
	if dirs == nil {
		dirs = []string{}
	}
	writeJSON(w, http.StatusOK, statsResponse{
		TotalFiles:     stats.TotalFiles,
		TotalSnapshots: stats.TotalSnapshots,
		TotalSize:      stats.TotalSize,
		WatchDirs:      dirs,
	})
}

func (s *Server) handleDatabaseDownload(w http.ResponseWriter, r *http.Request) {
	tmpDir := os.TempDir()
	snapshotPath, err := s.db.CreateDatabaseSnapshot(tmpDir)
	if err != nil {
		if strings.Contains(err.Error(), "insufficient disk space") {
			writeError(w, http.StatusInsufficientStorage, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer os.Remove(snapshotPath)

	f, err := os.Open(snapshotPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("open snapshot file: %w", err))
		return
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("stat snapshot file: %w", err))
		return
	}

	filename := fmt.Sprintf("history-%s.db", time.Now().Format("20060102-150405"))
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Type", "application/x-sqlite3")

	http.ServeContent(w, r, filename, fi.ModTime(), f)
}

func (s *Server) handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if err := s.db.DeleteFile(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, fmt.Errorf("file not found"))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSPA(w http.ResponseWriter, r *http.Request) {
	// Serve API paths that don't match will get 404
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeError(w, http.StatusNotFound, fmt.Errorf("endpoint not found"))
		return
	}

	if s.staticFS == nil {
		writeError(w, http.StatusNotFound, fmt.Errorf("static files not available"))
		return
	}

	// embed.FS rejects paths with ".." so path traversal is safe here
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}

	if _, err := fs.Stat(s.staticFS, path); err != nil {
		// SPA fallback: serve index.html for non-file paths
		path = "index.html"
		if _, err := fs.Stat(s.staticFS, path); err != nil {
			writeError(w, http.StatusNotFound, fmt.Errorf("index.html not found"))
			return
		}
	}

	http.ServeFileFS(w, r, s.staticFS, path)
}

// parseUUID extracts a UUID path parameter from the request and validates it.
func parseUUID(r *http.Request, name string) (string, error) {
	idStr := r.PathValue(name)
	if _, err := uuid.Parse(idStr); err != nil {
		return "", fmt.Errorf("invalid %s parameter: not a valid UUID", name)
	}
	return idStr, nil
}

// parseUUIDParam validates a UUID string from a query parameter.
func parseUUIDParam(value string, name string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("missing '%s' parameter", name)
	}
	if _, err := uuid.Parse(value); err != nil {
		return "", fmt.Errorf("invalid '%s' parameter: not a valid UUID", name)
	}
	return value, nil
}

type errorResponse struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("error encoding JSON response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, err error) {
	msg := err.Error()
	if status >= 500 {
		log.Printf("internal error: %v", err)
		msg = "internal server error"
	}
	writeJSON(w, status, errorResponse{Error: msg})
}
