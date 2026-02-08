package config

import (
	"encoding/json"
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
	if len(cfg.WatchSets) != 1 {
		t.Fatalf("WatchSets length = %d, want 1", len(cfg.WatchSets))
	}
	if cfg.WatchSets[0].DebounceSec != 3 {
		t.Errorf("WatchSets[0].DebounceSec = %d, want 3", cfg.WatchSets[0].DebounceSec)
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Port)
	}
	if cfg.WatchSets[0].MaxFileSize != 2097152 {
		t.Errorf("WatchSets[0].MaxFileSize = %d, want 2097152", cfg.WatchSets[0].MaxFileSize)
	}
	if cfg.WatchSets[0].MaxSnapshots != 100 {
		t.Errorf("WatchSets[0].MaxSnapshots = %d, want 100", cfg.WatchSets[0].MaxSnapshots)
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

	if len(cfg.WatchSets) != 1 {
		t.Fatalf("WatchSets length = %d, want 1", len(cfg.WatchSets))
	}
	ws := cfg.WatchSets[0]
	if ws.DebounceSec != 2 {
		t.Errorf("DebounceSec = %d, want 2", ws.DebounceSec)
	}
	if cfg.Port != 9876 {
		t.Errorf("Port = %d, want 9876", cfg.Port)
	}
	if ws.MaxFileSize != 1048576 {
		t.Errorf("MaxFileSize = %d, want 1048576", ws.MaxFileSize)
	}
	if ws.MaxSnapshots != 0 {
		t.Errorf("MaxSnapshots = %d, want 0", ws.MaxSnapshots)
	}
	if ws.Extensions != nil {
		t.Errorf("Extensions should be nil when not specified, got %v", ws.Extensions)
	}
	if len(ws.ExcludePatterns) == 0 {
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

func TestLoad_WatchSetsFormat(t *testing.T) {
	dir := t.TempDir()
	watchDir1 := filepath.Join(dir, "projects")
	watchDir2 := filepath.Join(dir, "documents")
	if err := os.Mkdir(watchDir1, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(watchDir2, 0o755); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(dir, "config.json")
	cfgData := map[string]any{
		"watchSets": []map[string]any{
			{
				"name":       "Projects",
				"dirs":       []string{watchDir1},
				"extensions": []string{".go", ".ts"},
				"debounceSec": 5,
			},
			{
				"name":         "Documents",
				"dirs":         []string{watchDir2},
				"extensions":   []string{".md", ".txt"},
				"maxSnapshots": 100,
			},
		},
		"dbPath": filepath.Join(dir, "history.db"),
	}
	data, err := json.Marshal(cfgData)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if len(cfg.WatchSets) != 2 {
		t.Fatalf("WatchSets length = %d, want 2", len(cfg.WatchSets))
	}

	ws0 := cfg.WatchSets[0]
	if ws0.Name != "Projects" {
		t.Errorf("WatchSets[0].Name = %s, want Projects", ws0.Name)
	}
	if len(ws0.Dirs) != 1 || ws0.Dirs[0] != watchDir1 {
		t.Errorf("WatchSets[0].Dirs = %v, want [%s]", ws0.Dirs, watchDir1)
	}
	if ws0.DebounceSec != 5 {
		t.Errorf("WatchSets[0].DebounceSec = %d, want 5", ws0.DebounceSec)
	}
	if ws0.MaxFileSize != 1048576 {
		t.Errorf("WatchSets[0].MaxFileSize = %d, want 1048576 (default)", ws0.MaxFileSize)
	}

	ws1 := cfg.WatchSets[1]
	if ws1.Name != "Documents" {
		t.Errorf("WatchSets[1].Name = %s, want Documents", ws1.Name)
	}
	if ws1.DebounceSec != 2 {
		t.Errorf("WatchSets[1].DebounceSec = %d, want 2 (default)", ws1.DebounceSec)
	}
	if ws1.MaxSnapshots != 100 {
		t.Errorf("WatchSets[1].MaxSnapshots = %d, want 100", ws1.MaxSnapshots)
	}

	// WatchDirs should be populated from WatchSets
	if len(cfg.WatchDirs) != 2 {
		t.Errorf("WatchDirs length = %d, want 2", len(cfg.WatchDirs))
	}
}

func TestLoad_WatchSetsClearsLegacyFields(t *testing.T) {
	dir := t.TempDir()
	watchDir := filepath.Join(dir, "watch")
	if err := os.Mkdir(watchDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(dir, "config.json")
	cfgData := map[string]any{
		"extensions":      []string{".legacy"},
		"excludePatterns": []string{"**/legacy/**"},
		"debounceSec":     99,
		"maxFileSize":     999,
		"maxSnapshots":    888,
		"watchSets": []map[string]any{
			{
				"name":       "SetA",
				"dirs":       []string{watchDir},
				"extensions": []string{".go"},
			},
		},
		"dbPath": filepath.Join(dir, "history.db"),
	}
	data, err := json.Marshal(cfgData)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.Extensions != nil {
		t.Errorf("Extensions should be nil after watchSets normalization, got %v", cfg.Extensions)
	}
	if cfg.ExcludePatterns != nil {
		t.Errorf("ExcludePatterns should be nil after watchSets normalization, got %v", cfg.ExcludePatterns)
	}
	if cfg.DebounceSec != 0 {
		t.Errorf("DebounceSec should be 0 after watchSets normalization, got %d", cfg.DebounceSec)
	}
	if cfg.MaxFileSize != 0 {
		t.Errorf("MaxFileSize should be 0 after watchSets normalization, got %d", cfg.MaxFileSize)
	}
	if cfg.MaxSnapshots != 0 {
		t.Errorf("MaxSnapshots should be 0 after watchSets normalization, got %d", cfg.MaxSnapshots)
	}
}

func TestLoad_WatchSetsIgnoresLegacyFields(t *testing.T) {
	dir := t.TempDir()
	watchDir := filepath.Join(dir, "watch")
	if err := os.Mkdir(watchDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(dir, "config.json")
	cfgData := map[string]any{
		"watchDirs":  []string{"/should/be/ignored"},
		"extensions": []string{".ignored"},
		"watchSets": []map[string]any{
			{
				"name":       "SetA",
				"dirs":       []string{watchDir},
				"extensions": []string{".go"},
			},
		},
		"dbPath": filepath.Join(dir, "history.db"),
	}
	data, err := json.Marshal(cfgData)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if len(cfg.WatchSets) != 1 {
		t.Fatalf("WatchSets length = %d, want 1", len(cfg.WatchSets))
	}
	if cfg.WatchSets[0].Extensions[0] != ".go" {
		t.Errorf("WatchSets[0].Extensions = %v, want [.go]", cfg.WatchSets[0].Extensions)
	}
	// WatchDirs should be set from WatchSets, not from legacy field
	if len(cfg.WatchDirs) != 1 || cfg.WatchDirs[0] != watchDir {
		t.Errorf("WatchDirs = %v, want [%s]", cfg.WatchDirs, watchDir)
	}
}

func TestLoad_WatchSetsDuplicateName(t *testing.T) {
	dir := t.TempDir()
	watchDir1 := filepath.Join(dir, "a")
	watchDir2 := filepath.Join(dir, "b")
	if err := os.Mkdir(watchDir1, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(watchDir2, 0o755); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(dir, "config.json")
	cfgData := map[string]any{
		"watchSets": []map[string]any{
			{"name": "Same", "dirs": []string{watchDir1}},
			{"name": "Same", "dirs": []string{watchDir2}},
		},
		"dbPath": filepath.Join(dir, "history.db"),
	}
	data, err := json.Marshal(cfgData)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = Load(cfgPath)
	if err == nil {
		t.Fatal("Load() should error on duplicate watchSet names")
	}
}

func TestLoad_WatchSetsDuplicateDir(t *testing.T) {
	dir := t.TempDir()
	watchDir := filepath.Join(dir, "shared")
	if err := os.Mkdir(watchDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(dir, "config.json")
	cfgData := map[string]any{
		"watchSets": []map[string]any{
			{"name": "SetA", "dirs": []string{watchDir}},
			{"name": "SetB", "dirs": []string{watchDir}},
		},
		"dbPath": filepath.Join(dir, "history.db"),
	}
	data, err := json.Marshal(cfgData)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = Load(cfgPath)
	if err == nil {
		t.Fatal("Load() should error on duplicate directories across watchSets")
	}
}

func TestLoad_WatchSetEmptyDirs(t *testing.T) {
	dir := t.TempDir()

	cfgPath := filepath.Join(dir, "config.json")
	cfgData := map[string]any{
		"watchSets": []map[string]any{
			{"name": "Empty", "dirs": []string{}},
		},
		"dbPath": filepath.Join(dir, "history.db"),
	}
	data, err := json.Marshal(cfgData)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	_, err = Load(cfgPath)
	if err == nil {
		t.Fatal("Load() should error on empty dirs in watchSet")
	}
}

func TestLoad_WatchSetAutoName(t *testing.T) {
	dir := t.TempDir()
	watchDir := filepath.Join(dir, "myproject")
	if err := os.Mkdir(watchDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(dir, "config.json")
	cfgData := map[string]any{
		"watchSets": []map[string]any{
			{"dirs": []string{watchDir}},
		},
		"dbPath": filepath.Join(dir, "history.db"),
	}
	data, err := json.Marshal(cfgData)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.WatchSets[0].Name != "myproject" {
		t.Errorf("auto name = %s, want myproject", cfg.WatchSets[0].Name)
	}
}

func TestLoad_LegacyConversionPreservesSettings(t *testing.T) {
	dir := t.TempDir()
	watchDir := filepath.Join(dir, "watch")
	if err := os.Mkdir(watchDir, 0o755); err != nil {
		t.Fatal(err)
	}

	cfgPath := filepath.Join(dir, "config.json")
	content := `{
		"watchDirs": ["` + watchDir + `"],
		"extensions": [".go", ".ts"],
		"excludePatterns": ["**/test/**"],
		"debounceSec": 5,
		"maxFileSize": 2097152,
		"maxSnapshots": 50,
		"dbPath": "` + filepath.Join(dir, "history.db") + `"
	}`
	if err := os.WriteFile(cfgPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if len(cfg.WatchSets) != 1 {
		t.Fatalf("WatchSets length = %d, want 1", len(cfg.WatchSets))
	}
	ws := cfg.WatchSets[0]
	if ws.Name != "watch" {
		t.Errorf("auto name = %s, want watch", ws.Name)
	}
	if len(ws.Extensions) != 2 || ws.Extensions[0] != ".go" {
		t.Errorf("Extensions = %v, want [.go .ts]", ws.Extensions)
	}
	if len(ws.ExcludePatterns) != 1 || ws.ExcludePatterns[0] != "**/test/**" {
		t.Errorf("ExcludePatterns = %v, want [**/test/**]", ws.ExcludePatterns)
	}
	if ws.DebounceSec != 5 {
		t.Errorf("DebounceSec = %d, want 5", ws.DebounceSec)
	}
	if ws.MaxFileSize != 2097152 {
		t.Errorf("MaxFileSize = %d, want 2097152", ws.MaxFileSize)
	}
	if ws.MaxSnapshots != 50 {
		t.Errorf("MaxSnapshots = %d, want 50", ws.MaxSnapshots)
	}
}

func TestAllWatchDirs(t *testing.T) {
	cfg := Config{
		WatchSets: []WatchSet{
			{Dirs: []string{"/a", "/b"}},
			{Dirs: []string{"/c"}},
		},
	}
	dirs := cfg.AllWatchDirs()
	if len(dirs) != 3 {
		t.Fatalf("AllWatchDirs() length = %d, want 3", len(dirs))
	}
	if dirs[0] != "/a" || dirs[1] != "/b" || dirs[2] != "/c" {
		t.Errorf("AllWatchDirs() = %v, want [/a /b /c]", dirs)
	}
}
