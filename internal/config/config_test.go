package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_ValidConfig(t *testing.T) {
	dir := t.TempDir()
	watchDir := filepath.Join(dir, "watch")
	if err := os.Mkdir(watchDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(dir, "config.json")
	content := `{
		"watchDirs": ["` + watchDir + `"],
		"debounceSec": 3,
		"port": 8080,
		"dbPath": "` + filepath.Join(dir, "history.db") + `",
		"maxFileSize": 2097152,
		"maxSnapshots": 100
	}`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if len(cfg.WatchDirs) != 1 || cfg.WatchDirs[0] != watchDir {
		t.Errorf("WatchDirs = %v, want [%s]", cfg.WatchDirs, watchDir)
	}
	if cfg.DebounceSec != 3 {
		t.Errorf("DebounceSec = %d, want 3", cfg.DebounceSec)
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
	if cfg.MaxFileSize != 2097152 {
		t.Errorf("MaxFileSize = %d, want 2097152", cfg.MaxFileSize)
	}
	if cfg.MaxSnapshots != 100 {
		t.Errorf("MaxSnapshots = %d, want 100", cfg.MaxSnapshots)
	}
}

func TestLoad_DefaultValues(t *testing.T) {
	dir := t.TempDir()
	watchDir := filepath.Join(dir, "watch")
	if err := os.Mkdir(watchDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(dir, "config.json")
	content := `{"watchDirs": ["` + watchDir + `"]}`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.DebounceSec != 2 {
		t.Errorf("DebounceSec = %d, want 2", cfg.DebounceSec)
	}
	if cfg.Port != 9876 {
		t.Errorf("Port = %d, want 9876", cfg.Port)
	}
	if cfg.MaxFileSize != 1048576 {
		t.Errorf("MaxFileSize = %d, want 1048576", cfg.MaxFileSize)
	}
	if cfg.MaxSnapshots != 0 {
		t.Errorf("MaxSnapshots = %d, want 0", cfg.MaxSnapshots)
	}
	if cfg.Extensions != nil {
		t.Errorf("Extensions should be nil when not specified, got %v", cfg.Extensions)
	}
	if len(cfg.ExcludePatterns) == 0 {
		t.Error("ExcludePatterns should have defaults")
	}
}

func TestLoad_EmptyWatchDirs(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	content := `{"watchDirs": []}`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("Load() should error on empty watchDirs")
	}
}

func TestLoad_MissingWatchDirs(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	content := `{}`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("Load() should error when watchDirs is missing")
	}
}

func TestLoad_NonExistentWatchDir(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	content := `{"watchDirs": ["/nonexistent/path"]}`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("Load() should error on non-existent watchDir")
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte("{invalid}"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("Load() should error on invalid JSON")
	}
}

func TestLoad_FileNotFound(t *testing.T) {
	_, err := Load("/nonexistent/config.json")
	if err == nil {
		t.Fatal("Load() should error on missing file")
	}
}

func TestLoad_InvalidPort(t *testing.T) {
	dir := t.TempDir()
	watchDir := filepath.Join(dir, "watch")
	if err := os.Mkdir(watchDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(dir, "config.json")
	content := `{"watchDirs": ["` + watchDir + `"], "port": 99999}`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("Load() should error on invalid port")
	}
}

func TestLoad_TildeExpansion(t *testing.T) {
	dir := t.TempDir()
	watchDir := filepath.Join(dir, "watch")
	if err := os.Mkdir(watchDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(dir, "config.json")
	content := `{"watchDirs": ["` + watchDir + `"], "dbPath": "~/test.db"}`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, "test.db")
	if cfg.DBPath != expected {
		t.Errorf("DBPath = %s, want %s", cfg.DBPath, expected)
	}
}

func TestLoad_BasicAuthValid(t *testing.T) {
	dir := t.TempDir()
	watchDir := filepath.Join(dir, "watch")
	if err := os.Mkdir(watchDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(dir, "config.json")
	content := `{
		"watchDirs": ["` + watchDir + `"],
		"dbPath": "` + filepath.Join(dir, "history.db") + `",
		"basicAuth": {"username": "admin", "password": "secret"}
	}`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.BasicAuth == nil {
		t.Fatal("BasicAuth should not be nil")
	}
	if cfg.BasicAuth.Username != "admin" {
		t.Errorf("BasicAuth.Username = %s, want admin", cfg.BasicAuth.Username)
	}
	if cfg.BasicAuth.Password != "secret" {
		t.Errorf("BasicAuth.Password = %s, want secret", cfg.BasicAuth.Password)
	}
}

func TestLoad_BasicAuthMissingUsername(t *testing.T) {
	dir := t.TempDir()
	watchDir := filepath.Join(dir, "watch")
	if err := os.Mkdir(watchDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(dir, "config.json")
	content := `{
		"watchDirs": ["` + watchDir + `"],
		"dbPath": "` + filepath.Join(dir, "history.db") + `",
		"basicAuth": {"username": "", "password": "secret"}
	}`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("Load() should error when basicAuth.username is empty")
	}
}

func TestLoad_BasicAuthMissingPassword(t *testing.T) {
	dir := t.TempDir()
	watchDir := filepath.Join(dir, "watch")
	if err := os.Mkdir(watchDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(dir, "config.json")
	content := `{
		"watchDirs": ["` + watchDir + `"],
		"dbPath": "` + filepath.Join(dir, "history.db") + `",
		"basicAuth": {"username": "admin", "password": ""}
	}`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("Load() should error when basicAuth.password is empty")
	}
}

func TestLoad_BasicAuthOmitted(t *testing.T) {
	dir := t.TempDir()
	watchDir := filepath.Join(dir, "watch")
	if err := os.Mkdir(watchDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(dir, "config.json")
	content := `{
		"watchDirs": ["` + watchDir + `"],
		"dbPath": "` + filepath.Join(dir, "history.db") + `"
	}`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.BasicAuth != nil {
		t.Errorf("BasicAuth should be nil when not configured, got %+v", cfg.BasicAuth)
	}
}

func TestLoad_WatchDirIsFile(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "notadir")
	if err := os.WriteFile(filePath, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(dir, "config.json")
	content := `{"watchDirs": ["` + filePath + `"]}`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Load(cfgPath)
	if err == nil {
		t.Fatal("Load() should error when watchDir is a file")
	}
}
