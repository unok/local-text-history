package db

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/klauspost/compress/zstd"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/sys/unix"
)

// File represents a tracked file record.
type File struct {
	ID      string `json:"id"`
	Path    string `json:"path"`
	Created int64  `json:"created"`
	Updated int64  `json:"updated"`
}

// Snapshot represents a file snapshot record.
type Snapshot struct {
	ID        string `json:"id"`
	FileID    string `json:"fileId"`
	Content   []byte `json:"-"`
	Size      int64  `json:"size"`
	Hash      string `json:"hash"`
	Timestamp int64  `json:"timestamp"`
}

// HistoryEntry represents a recent snapshot with file path information.
type HistoryEntry struct {
	SnapshotID string `json:"snapshotId"`
	FileID     string `json:"fileId"`
	FilePath   string `json:"filePath"`
	Size       int64  `json:"size"`
	Hash       string `json:"hash"`
	Timestamp  int64  `json:"timestamp"`
}

// Rename represents a file rename record.
type Rename struct {
	ID        string `json:"id"`
	OldFileID string `json:"oldFileId"`
	NewFileID string `json:"newFileId"`
	OldPath   string `json:"oldPath"`
	NewPath   string `json:"newPath"`
	Timestamp int64  `json:"timestamp"`
}

// Stats holds aggregate statistics.
type Stats struct {
	TotalFiles     int   `json:"totalFiles"`
	TotalSnapshots int   `json:"totalSnapshots"`
	TotalSize      int64 `json:"totalSize"`
}

// DB wraps a SQLite database connection for file history operations.
type DB struct {
	db           *sql.DB
	encoder      *zstd.Encoder
	decoder      *zstd.Decoder
	maxSnapshots int
}

// New opens a SQLite database at the given path, enables WAL mode and
// foreign keys, creates the schema, and returns a DB instance.
func New(dbPath string, maxSnapshots int) (*DB, error) {
	sqlDB, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	if _, err := sqlDB.Exec("PRAGMA journal_mode = WAL"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("setting WAL mode: %w", err)
	}
	if _, err := sqlDB.Exec("PRAGMA synchronous = NORMAL"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("setting synchronous mode: %w", err)
	}

	if err := createSchema(sqlDB); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("creating schema: %w", err)
	}

	if err := migrateIfNeeded(sqlDB); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("migrating schema: %w", err)
	}

	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("creating zstd encoder: %w", err)
	}

	decoder, err := zstd.NewReader(nil)
	if err != nil {
		sqlDB.Close()
		encoder.Close()
		return nil, fmt.Errorf("creating zstd decoder: %w", err)
	}

	return &DB{
		db:           sqlDB,
		encoder:      encoder,
		decoder:      decoder,
		maxSnapshots: maxSnapshots,
	}, nil
}

func createSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS files (
		id       TEXT PRIMARY KEY,
		path     TEXT NOT NULL UNIQUE,
		created  INTEGER NOT NULL DEFAULT (unixepoch()),
		updated  INTEGER NOT NULL DEFAULT (unixepoch())
	);

	CREATE TABLE IF NOT EXISTS snapshots (
		id        TEXT PRIMARY KEY,
		file_id   TEXT NOT NULL REFERENCES files(id) ON DELETE CASCADE,
		content   BLOB NOT NULL,
		size      INTEGER NOT NULL,
		hash      TEXT NOT NULL,
		timestamp INTEGER NOT NULL DEFAULT (unixepoch())
	);

	CREATE INDEX IF NOT EXISTS idx_snapshots_file_ts ON snapshots(file_id, timestamp DESC);
	CREATE INDEX IF NOT EXISTS idx_snapshots_timestamp ON snapshots(timestamp DESC, id DESC);
	CREATE INDEX IF NOT EXISTS idx_files_path ON files(path);

	CREATE TABLE IF NOT EXISTS renames (
		id          TEXT PRIMARY KEY,
		old_file_id TEXT NOT NULL REFERENCES files(id) ON DELETE CASCADE,
		new_file_id TEXT NOT NULL REFERENCES files(id) ON DELETE CASCADE,
		old_path    TEXT NOT NULL,
		new_path    TEXT NOT NULL,
		timestamp   INTEGER NOT NULL DEFAULT (unixepoch())
	);

	CREATE INDEX IF NOT EXISTS idx_renames_old_file ON renames(old_file_id, timestamp DESC);
	CREATE INDEX IF NOT EXISTS idx_renames_new_file ON renames(new_file_id, timestamp DESC);
	`
	_, err := db.Exec(schema)
	return err
}

// migrateIfNeeded checks the files table schema and migrates from
// INTEGER PRIMARY KEY to TEXT PRIMARY KEY (UUIDv7) if needed.
func migrateIfNeeded(db *sql.DB) error {
	needsMigration, err := needsSchemaMigration(db)
	if err != nil {
		return err
	}
	if !needsMigration {
		return nil
	}

	// Disable foreign keys during migration
	if _, err := db.Exec("PRAGMA foreign_keys = OFF"); err != nil {
		return fmt.Errorf("disabling foreign keys: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("beginning migration transaction: %w", err)
	}
	defer tx.Rollback()

	migrationSQL := `
	-- Create new tables with TEXT PRIMARY KEY
	CREATE TABLE files_new (
		id       TEXT PRIMARY KEY,
		path     TEXT NOT NULL UNIQUE,
		created  INTEGER NOT NULL DEFAULT (unixepoch()),
		updated  INTEGER NOT NULL DEFAULT (unixepoch())
	);

	CREATE TABLE snapshots_new (
		id        TEXT PRIMARY KEY,
		file_id   TEXT NOT NULL REFERENCES files_new(id) ON DELETE CASCADE,
		content   BLOB NOT NULL,
		size      INTEGER NOT NULL,
		hash      TEXT NOT NULL,
		timestamp INTEGER NOT NULL DEFAULT (unixepoch())
	);

	-- Create temporary mapping table for old INTEGER IDs to new UUIDv7 IDs
	CREATE TEMPORARY TABLE id_mapping (
		old_id INTEGER NOT NULL,
		new_id TEXT NOT NULL
	);
	`
	if _, err := tx.Exec(migrationSQL); err != nil {
		return fmt.Errorf("creating migration tables: %w", err)
	}

	// Migrate files: generate UUIDv7 for each row and record the mapping
	fileRows, err := tx.Query("SELECT id, path, created, updated FROM files")
	if err != nil {
		return fmt.Errorf("reading files: %w", err)
	}

	type fileMapping struct {
		oldID   int64
		newID   string
		path    string
		created int64
		updated int64
	}
	var fileMappings []fileMapping

	for fileRows.Next() {
		var fm fileMapping
		if err := fileRows.Scan(&fm.oldID, &fm.path, &fm.created, &fm.updated); err != nil {
			fileRows.Close()
			return fmt.Errorf("scanning file row: %w", err)
		}
		fm.newID = newUUIDv7()
		fileMappings = append(fileMappings, fm)
	}
	if err := fileRows.Err(); err != nil {
		return fmt.Errorf("iterating file rows: %w", err)
	}
	fileRows.Close()

	for _, fm := range fileMappings {
		if _, err := tx.Exec(
			"INSERT INTO files_new (id, path, created, updated) VALUES (?, ?, ?, ?)",
			fm.newID, fm.path, fm.created, fm.updated,
		); err != nil {
			return fmt.Errorf("inserting migrated file: %w", err)
		}
		if _, err := tx.Exec(
			"INSERT INTO id_mapping (old_id, new_id) VALUES (?, ?)",
			fm.oldID, fm.newID,
		); err != nil {
			return fmt.Errorf("inserting id mapping: %w", err)
		}
	}

	// Migrate snapshots: generate UUIDv7 for each snapshot and map file_id
	snapshotRows, err := tx.Query("SELECT id, file_id, content, size, hash, timestamp FROM snapshots")
	if err != nil {
		return fmt.Errorf("reading snapshots: %w", err)
	}

	type snapshotData struct {
		oldFileID int64
		content   []byte
		size      int64
		hash      string
		timestamp int64
	}
	var snapshots []snapshotData

	for snapshotRows.Next() {
		var oldID int64
		var sd snapshotData
		if err := snapshotRows.Scan(&oldID, &sd.oldFileID, &sd.content, &sd.size, &sd.hash, &sd.timestamp); err != nil {
			snapshotRows.Close()
			return fmt.Errorf("scanning snapshot row: %w", err)
		}
		snapshots = append(snapshots, sd)
	}
	if err := snapshotRows.Err(); err != nil {
		return fmt.Errorf("iterating snapshot rows: %w", err)
	}
	snapshotRows.Close()

	// Build old_id -> new_id lookup from mapping table
	mappingRows, err := tx.Query("SELECT old_id, new_id FROM id_mapping")
	if err != nil {
		return fmt.Errorf("reading id mapping: %w", err)
	}
	idMap := make(map[int64]string)
	for mappingRows.Next() {
		var oldID int64
		var newID string
		if err := mappingRows.Scan(&oldID, &newID); err != nil {
			mappingRows.Close()
			return fmt.Errorf("scanning id mapping: %w", err)
		}
		idMap[oldID] = newID
	}
	if err := mappingRows.Err(); err != nil {
		return fmt.Errorf("iterating id mapping rows: %w", err)
	}
	mappingRows.Close()

	for _, sd := range snapshots {
		newFileID, ok := idMap[sd.oldFileID]
		if !ok {
			return fmt.Errorf("no mapping for old file_id %d", sd.oldFileID)
		}
		newSnapID := newUUIDv7()
		if _, err := tx.Exec(
			"INSERT INTO snapshots_new (id, file_id, content, size, hash, timestamp) VALUES (?, ?, ?, ?, ?, ?)",
			newSnapID, newFileID, sd.content, sd.size, sd.hash, sd.timestamp,
		); err != nil {
			return fmt.Errorf("inserting migrated snapshot: %w", err)
		}
	}

	// Drop old tables and rename new ones
	replaceSQL := `
	DROP TABLE snapshots;
	DROP TABLE files;
	ALTER TABLE files_new RENAME TO files;
	ALTER TABLE snapshots_new RENAME TO snapshots;

	CREATE INDEX IF NOT EXISTS idx_snapshots_file_ts ON snapshots(file_id, timestamp DESC);
	CREATE INDEX IF NOT EXISTS idx_snapshots_timestamp ON snapshots(timestamp DESC, id DESC);
	CREATE INDEX IF NOT EXISTS idx_files_path ON files(path);
	`
	if _, err := tx.Exec(replaceSQL); err != nil {
		return fmt.Errorf("replacing tables: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing migration: %w", err)
	}

	// Re-enable foreign keys and verify integrity
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return fmt.Errorf("re-enabling foreign keys: %w", err)
	}

	rows, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		return fmt.Errorf("checking foreign keys: %w", err)
	}
	defer rows.Close()
	if rows.Next() {
		return fmt.Errorf("foreign key integrity check failed after migration")
	}

	return nil
}

// needsSchemaMigration checks the files table's id column type.
// Returns true if the type is INTEGER (old schema), false if TEXT (new schema).
func needsSchemaMigration(db *sql.DB) (bool, error) {
	rows, err := db.Query("PRAGMA table_info(files)")
	if err != nil {
		return false, fmt.Errorf("reading table info: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			return false, fmt.Errorf("scanning column info: %w", err)
		}
		if name == "id" {
			return colType == "INTEGER", nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("iterating column info: %w", err)
	}

	// Table doesn't exist or has no id column — no migration needed
	return false, nil
}

// Close closes the database connection and releases zstd resources.
func (d *DB) Close() error {
	d.encoder.Close()
	d.decoder.Close()
	return d.db.Close()
}

func newUUIDv7() string {
	return uuid.Must(uuid.NewV7()).String()
}

// SaveSnapshot saves a file snapshot. It returns false if the content
// hash matches the latest snapshot (duplicate skip).
func (d *DB) SaveSnapshot(filePath string, content []byte) (bool, error) {
	tx, err := d.db.Begin()
	if err != nil {
		return false, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	saved, err := d.saveSnapshotInTx(tx, filePath, content)
	if err != nil {
		return false, err
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("committing transaction: %w", err)
	}
	return saved, nil
}

// SaveSnapshotBatch saves multiple file snapshots in a single transaction.
// Returns a saved flag and error for each input item.
func (d *DB) SaveSnapshotBatch(filePaths []string, contents [][]byte) ([]bool, []error) {
	n := len(filePaths)
	saved := make([]bool, n)
	errs := make([]error, n)

	tx, err := d.db.Begin()
	if err != nil {
		for i := range errs {
			errs[i] = fmt.Errorf("beginning transaction: %w", err)
		}
		return saved, errs
	}
	defer tx.Rollback()

	for i := range n {
		saved[i], errs[i] = d.saveSnapshotInTx(tx, filePaths[i], contents[i])
	}

	if err := tx.Commit(); err != nil {
		for i := range errs {
			if errs[i] == nil && saved[i] {
				errs[i] = fmt.Errorf("committing transaction: %w", err)
				saved[i] = false
			}
		}
	}

	return saved, errs
}

// saveSnapshotInTx performs the snapshot save logic within an existing transaction.
func (d *DB) saveSnapshotInTx(tx *sql.Tx, filePath string, content []byte) (bool, error) {
	hash := sha256sum(content)

	// Check if file already exists and get its ID + latest snapshot hash
	var fileID string
	var lastHash sql.NullString
	err := tx.QueryRow(
		`SELECT f.id, (
			SELECT hash FROM snapshots WHERE file_id = f.id ORDER BY timestamp DESC LIMIT 1
		 ) FROM files f WHERE f.path = ?`,
		filePath,
	).Scan(&fileID, &lastHash)
	if err != nil && err != sql.ErrNoRows {
		return false, fmt.Errorf("checking existing file: %w", err)
	}

	// Skip if content hasn't changed
	if lastHash.Valid && lastHash.String == hash {
		return false, nil
	}

	now := time.Now().Unix()

	if err == sql.ErrNoRows {
		// New file: insert with UUIDv7
		fileID = newUUIDv7()
		_, err = tx.Exec(
			`INSERT INTO files (id, path, created, updated) VALUES (?, ?, ?, ?)`,
			fileID, filePath, now, now,
		)
		if err != nil {
			return false, fmt.Errorf("inserting file: %w", err)
		}
	} else {
		// Existing file with changed content: update timestamp
		_, err = tx.Exec(`UPDATE files SET updated = ? WHERE id = ?`, now, fileID)
		if err != nil {
			return false, fmt.Errorf("updating file: %w", err)
		}
	}

	// Compress and save with UUIDv7
	compressed := d.encoder.EncodeAll(content, nil)
	snapshotID := newUUIDv7()
	_, err = tx.Exec(
		`INSERT INTO snapshots (id, file_id, content, size, hash, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		snapshotID, fileID, compressed, len(content), hash, now,
	)
	if err != nil {
		return false, fmt.Errorf("inserting snapshot: %w", err)
	}

	// Enforce maxSnapshots limit
	if d.maxSnapshots > 0 {
		_, err = tx.Exec(
			`DELETE FROM snapshots WHERE file_id = ? AND id NOT IN (
				SELECT id FROM snapshots WHERE file_id = ? ORDER BY timestamp DESC LIMIT ?
			)`,
			fileID, fileID, d.maxSnapshots,
		)
		if err != nil {
			return false, fmt.Errorf("pruning old snapshots: %w", err)
		}
	}

	return true, nil
}

// SearchFiles searches for files whose path contains the query string.
func (d *DB) SearchFiles(query string, limit, offset int) ([]File, error) {
	rows, err := d.db.Query(
		`SELECT id, path, created, updated FROM files
		 WHERE path LIKE '%' || ? || '%'
		 ORDER BY updated DESC
		 LIMIT ? OFFSET ?`,
		query, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("searching files: %w", err)
	}
	defer rows.Close()

	var files []File
	for rows.Next() {
		var f File
		if err := rows.Scan(&f.ID, &f.Path, &f.Created, &f.Updated); err != nil {
			return nil, fmt.Errorf("scanning file: %w", err)
		}
		files = append(files, f)
	}
	return files, rows.Err()
}

// GetFile returns a single file by ID.
func (d *DB) GetFile(id string) (File, error) {
	var f File
	err := d.db.QueryRow(
		`SELECT id, path, created, updated FROM files WHERE id = ?`, id,
	).Scan(&f.ID, &f.Path, &f.Created, &f.Updated)
	if err != nil {
		return File{}, fmt.Errorf("getting file: %w", err)
	}
	return f, nil
}

// GetSnapshots returns all snapshots for a file, newest first.
func (d *DB) GetSnapshots(fileID string) ([]Snapshot, error) {
	rows, err := d.db.Query(
		`SELECT id, file_id, size, hash, timestamp FROM snapshots
		 WHERE file_id = ?
		 ORDER BY timestamp DESC`,
		fileID,
	)
	if err != nil {
		return nil, fmt.Errorf("getting snapshots: %w", err)
	}
	defer rows.Close()

	var snapshots []Snapshot
	for rows.Next() {
		var s Snapshot
		if err := rows.Scan(&s.ID, &s.FileID, &s.Size, &s.Hash, &s.Timestamp); err != nil {
			return nil, fmt.Errorf("scanning snapshot: %w", err)
		}
		snapshots = append(snapshots, s)
	}
	return snapshots, rows.Err()
}

// GetSnapshot returns a single snapshot by ID, including decompressed content.
func (d *DB) GetSnapshot(id string) (Snapshot, error) {
	var s Snapshot
	var compressed []byte
	err := d.db.QueryRow(
		`SELECT id, file_id, content, size, hash, timestamp FROM snapshots WHERE id = ?`, id,
	).Scan(&s.ID, &s.FileID, &compressed, &s.Size, &s.Hash, &s.Timestamp)
	if err != nil {
		return Snapshot{}, fmt.Errorf("getting snapshot: %w", err)
	}

	content, err := d.decoder.DecodeAll(compressed, nil)
	if err != nil {
		return Snapshot{}, fmt.Errorf("decompressing snapshot: %w", err)
	}
	s.Content = content
	return s, nil
}

// DeleteFile deletes a file and all its snapshots (CASCADE).
func (d *DB) DeleteFile(id string) error {
	result, err := d.db.Exec(`DELETE FROM files WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting file: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking rows affected: %w", err)
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// GetStats returns aggregate statistics.
func (d *DB) GetStats() (Stats, error) {
	var stats Stats
	err := d.db.QueryRow(`SELECT COUNT(*) FROM files`).Scan(&stats.TotalFiles)
	if err != nil {
		return Stats{}, fmt.Errorf("counting files: %w", err)
	}
	err = d.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(size), 0) FROM snapshots`).Scan(
		&stats.TotalSnapshots, &stats.TotalSize,
	)
	if err != nil {
		return Stats{}, fmt.Errorf("counting snapshots: %w", err)
	}
	return stats, nil
}

// GetRecentSnapshots returns the most recent snapshots across all files,
// joined with their file path, ordered by timestamp descending.
func (d *DB) GetRecentSnapshots(limit int) ([]HistoryEntry, error) {
	rows, err := d.db.Query(
		`SELECT s.id, s.file_id, f.path, s.size, s.hash, s.timestamp
		 FROM snapshots s
		 JOIN files f ON s.file_id = f.id
		 ORDER BY s.timestamp DESC, s.id DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("getting recent snapshots: %w", err)
	}
	defer rows.Close()

	var entries []HistoryEntry
	for rows.Next() {
		var e HistoryEntry
		if err := rows.Scan(&e.SnapshotID, &e.FileID, &e.FilePath, &e.Size, &e.Hash, &e.Timestamp); err != nil {
			return nil, fmt.Errorf("scanning history entry: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// DatabaseSize returns the estimated database size in bytes using PRAGMA values.
func (d *DB) DatabaseSize() (int64, error) {
	var pageCount, pageSize int64
	if err := d.db.QueryRow("PRAGMA page_count").Scan(&pageCount); err != nil {
		return 0, fmt.Errorf("querying page_count: %w", err)
	}
	if err := d.db.QueryRow("PRAGMA page_size").Scan(&pageSize); err != nil {
		return 0, fmt.Errorf("querying page_size: %w", err)
	}
	return pageCount * pageSize, nil
}

// CreateDatabaseSnapshot creates a consistent snapshot of the database using VACUUM INTO.
// It writes the snapshot to a temporary file and returns the file path.
// The caller is responsible for removing the file after use.
func (d *DB) CreateDatabaseSnapshot(tmpDir string) (string, error) {
	dbSize, err := d.DatabaseSize()
	if err != nil {
		return "", fmt.Errorf("getting database size: %w", err)
	}

	var stat unix.Statfs_t
	if err := unix.Statfs(tmpDir, &stat); err != nil {
		return "", fmt.Errorf("checking disk space: %w", err)
	}
	availableBytes := uint64(stat.Bavail) * uint64(stat.Bsize)
	if dbSize < 0 || uint64(dbSize) > availableBytes {
		return "", fmt.Errorf("insufficient disk space: need %d bytes, available %d bytes", dbSize, availableBytes)
	}

	tmpFile, err := os.CreateTemp(tmpDir, "history-snapshot-*.db")
	if err != nil {
		return "", fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	// Remove the empty file so VACUUM INTO can create it
	os.Remove(tmpPath)

	escapedPath := strings.ReplaceAll(tmpPath, "'", "''")
	if _, err := d.db.Exec(fmt.Sprintf("VACUUM INTO '%s'", escapedPath)); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("vacuum into: %w", err)
	}

	return tmpPath, nil
}

// SaveRename records a file rename event. It looks up the old file by path
// and creates a new file record for the new path if one doesn't exist.
// Returns the new file's ID.
func (d *DB) SaveRename(oldPath, newPath string) (string, error) {
	tx, err := d.db.Begin()
	if err != nil {
		return "", fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	// Look up old file
	var oldFileID string
	err = tx.QueryRow(`SELECT id FROM files WHERE path = ?`, oldPath).Scan(&oldFileID)
	if err != nil {
		return "", fmt.Errorf("looking up old file %q: %w", oldPath, err)
	}

	now := time.Now().Unix()

	// Look up or create new file
	var newFileID string
	err = tx.QueryRow(`SELECT id FROM files WHERE path = ?`, newPath).Scan(&newFileID)
	if err == sql.ErrNoRows {
		newFileID = newUUIDv7()
		_, err = tx.Exec(
			`INSERT INTO files (id, path, created, updated) VALUES (?, ?, ?, ?)`,
			newFileID, newPath, now, now,
		)
		if err != nil {
			return "", fmt.Errorf("inserting new file: %w", err)
		}
	} else if err != nil {
		return "", fmt.Errorf("looking up new file %q: %w", newPath, err)
	}

	// Record the rename
	renameID := newUUIDv7()
	_, err = tx.Exec(
		`INSERT INTO renames (id, old_file_id, new_file_id, old_path, new_path, timestamp)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		renameID, oldFileID, newFileID, oldPath, newPath, now,
	)
	if err != nil {
		return "", fmt.Errorf("inserting rename: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return "", fmt.Errorf("committing transaction: %w", err)
	}
	return newFileID, nil
}

// GetRenames returns all rename records associated with the given file ID,
// either as source (old_file_id) or destination (new_file_id), ordered by timestamp.
func (d *DB) GetRenames(fileID string) ([]Rename, error) {
	rows, err := d.db.Query(
		`SELECT id, old_file_id, new_file_id, old_path, new_path, timestamp
		 FROM renames
		 WHERE old_file_id = ? OR new_file_id = ?
		 ORDER BY timestamp ASC, id ASC`,
		fileID, fileID,
	)
	if err != nil {
		return nil, fmt.Errorf("getting renames: %w", err)
	}
	defer rows.Close()

	var renames []Rename
	for rows.Next() {
		var r Rename
		if err := rows.Scan(&r.ID, &r.OldFileID, &r.NewFileID, &r.OldPath, &r.NewPath, &r.Timestamp); err != nil {
			return nil, fmt.Errorf("scanning rename: %w", err)
		}
		renames = append(renames, r)
	}
	return renames, rows.Err()
}

func sha256sum(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
