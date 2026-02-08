package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// BasicAuthConfig holds Basic authentication credentials.
type BasicAuthConfig struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// WatchSet defines a named group of directories with shared monitoring settings.
type WatchSet struct {
	Name            string   `json:"name"`
	Dirs            []string `json:"dirs"`
	Extensions      []string `json:"extensions"`
	ExcludePatterns []string `json:"excludePatterns"`
	DebounceSec     int      `json:"debounceSec"`
	MaxFileSize     int64    `json:"maxFileSize"`
	MaxSnapshots    int      `json:"maxSnapshots"`
}

// Config holds all application configuration.
type Config struct {
	// Legacy fields for JSON deserialization only.
	// After normalizeWatchSets, these are cleared; use WatchSets[] instead.
	WatchDirs       []string `json:"watchDirs,omitempty"`
	Extensions      []string `json:"extensions,omitempty"`
	ExcludePatterns []string `json:"excludePatterns,omitempty"`
	DebounceSec     int      `json:"debounceSec"`
	MaxFileSize     int64    `json:"maxFileSize"`
	MaxSnapshots    int      `json:"maxSnapshots"`

	// New: named watch sets with per-set configuration
	WatchSets []WatchSet `json:"watchSets,omitempty"`

	// Global settings
	BindAddress string           `json:"bindAddress"`
	Port        int              `json:"port"`
	DBPath      string           `json:"dbPath"`
	BasicAuth   *BasicAuthConfig `json:"basicAuth,omitempty"`
}

// AllWatchDirs returns all directories from all WatchSets flattened.
func (c *Config) AllWatchDirs() []string {
	var dirs []string
	for _, ws := range c.WatchSets {
		dirs = append(dirs, ws.Dirs...)
	}
	return dirs
}

// Load reads a JSON config file and returns a validated Config.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("reading config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing config file: %w", err)
	}

	applyDefaults(&cfg)

	expanded, err := expandPath(cfg.DBPath)
	if err != nil {
		return Config{}, fmt.Errorf("expanding dbPath: %w", err)
	}
	cfg.DBPath = expanded

	if err := validate(cfg); err != nil {
		return Config{}, fmt.Errorf("validating config: %w", err)
	}

	return cfg, nil
}

func applyDefaults(cfg *Config) {
	if cfg.BindAddress == "" {
		cfg.BindAddress = "0.0.0.0"
	}
	if cfg.Port == 0 {
		cfg.Port = 9876
	}
	if cfg.DBPath == "" {
		cfg.DBPath = "~/.local/share/file-history/history.db"
	}

	normalizeWatchSets(cfg)
}

// normalizeWatchSets converts legacy watchDirs format to WatchSets,
// or applies defaults to existing WatchSets.
func normalizeWatchSets(cfg *Config) {
	if len(cfg.WatchSets) > 0 {
		for i := range cfg.WatchSets {
			applyWatchSetDefaults(&cfg.WatchSets[i])
		}
		cfg.WatchDirs = cfg.AllWatchDirs()
		return
	}

	if len(cfg.WatchDirs) > 0 {
		ws := WatchSet{
			Name:            defaultWatchSetName(cfg.WatchDirs),
			Dirs:            cfg.WatchDirs,
			Extensions:      cfg.Extensions,
			ExcludePatterns: cfg.ExcludePatterns,
			DebounceSec:     cfg.DebounceSec,
			MaxFileSize:     cfg.MaxFileSize,
			MaxSnapshots:    cfg.MaxSnapshots,
		}
		applyWatchSetDefaults(&ws)
		cfg.WatchSets = []WatchSet{ws}
	}

	// Clear legacy fields after conversion to prevent accidental use.
	// All per-set values are now in cfg.WatchSets[].
	cfg.Extensions = nil
	cfg.ExcludePatterns = nil
	cfg.DebounceSec = 0
	cfg.MaxFileSize = 0
	cfg.MaxSnapshots = 0
}

func applyWatchSetDefaults(ws *WatchSet) {
	if ws.DebounceSec == 0 {
		ws.DebounceSec = 2
	}
	if ws.MaxFileSize == 0 {
		ws.MaxFileSize = 1048576 // 1MB
	}
	if ws.ExcludePatterns == nil {
		ws.ExcludePatterns = defaultExcludePatterns()
	}
	if ws.Name == "" {
		ws.Name = defaultWatchSetName(ws.Dirs)
	}
}

func defaultWatchSetName(dirs []string) string {
	if len(dirs) == 0 {
		return "default"
	}
	return filepath.Base(dirs[0])
}

func validate(cfg Config) error {
	if len(cfg.WatchSets) == 0 {
		return errors.New("watchSets (or watchDirs) must not be empty")
	}

	if cfg.Port < 1 || cfg.Port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}
	if cfg.BasicAuth != nil {
		if cfg.BasicAuth.Username == "" {
			return errors.New("basicAuth.username must not be empty when basicAuth is configured")
		}
		if cfg.BasicAuth.Password == "" {
			return errors.New("basicAuth.password must not be empty when basicAuth is configured")
		}
	}

	nameSet := make(map[string]struct{})
	dirSet := make(map[string]struct{})

	for i, ws := range cfg.WatchSets {
		if len(ws.Dirs) == 0 {
			return fmt.Errorf("watchSets[%d].dirs must not be empty", i)
		}
		if ws.DebounceSec < 1 {
			return fmt.Errorf("watchSets[%d].debounceSec must be >= 1", i)
		}
		if ws.MaxFileSize < 1 {
			return fmt.Errorf("watchSets[%d].maxFileSize must be >= 1", i)
		}
		if ws.MaxSnapshots < 0 {
			return fmt.Errorf("watchSets[%d].maxSnapshots must be >= 0", i)
		}

		if _, exists := nameSet[ws.Name]; exists {
			return fmt.Errorf("duplicate watchSet name %q", ws.Name)
		}
		nameSet[ws.Name] = struct{}{}

		for _, dir := range ws.Dirs {
			if _, exists := dirSet[dir]; exists {
				return fmt.Errorf("directory %q appears in multiple watchSets", dir)
			}
			dirSet[dir] = struct{}{}

			info, err := os.Stat(dir)
			if err != nil {
				return fmt.Errorf("watchSet %q dir %q: %w", ws.Name, dir, err)
			}
			if !info.IsDir() {
				return fmt.Errorf("watchSet %q dir %q is not a directory", ws.Name, dir)
			}
		}
	}

	return nil
}

// expandPath replaces a leading ~ with the user's home directory.
func expandPath(path string) (string, error) {
	if !strings.HasPrefix(path, "~") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(home, path[1:]), nil
}

func defaultExcludePatterns() []string {
	return []string{
		"**/node_modules/**",
		"**/.git/**",
		"**/vendor/**",
		"**/dist/**",
		"**/build/**",
		"**/.next/**",
		"**/__pycache__/**",
		"**/target/**",
		"**/*.min.js",
		"**/*.min.css",
		"**/*.lock",
		"**/package-lock.json",
		"**/pnpm-lock.yaml",
	}
}
