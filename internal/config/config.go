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

// Config holds all application configuration.
type Config struct {
	WatchDirs       []string         `json:"watchDirs"`
	DebounceSec     int              `json:"debounceSec"`
	BindAddress     string           `json:"bindAddress"`
	Port            int              `json:"port"`
	DBPath          string           `json:"dbPath"`
	Extensions      []string         `json:"extensions"`
	ExcludePatterns []string         `json:"excludePatterns"`
	MaxFileSize     int64            `json:"maxFileSize"`
	MaxSnapshots    int              `json:"maxSnapshots"`
	BasicAuth       *BasicAuthConfig `json:"basicAuth,omitempty"`
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
	if cfg.DebounceSec == 0 {
		cfg.DebounceSec = 2
	}
	if cfg.BindAddress == "" {
		cfg.BindAddress = "0.0.0.0"
	}
	if cfg.Port == 0 {
		cfg.Port = 9876
	}
	if cfg.DBPath == "" {
		cfg.DBPath = "~/.local/share/file-history/history.db"
	}
	if cfg.MaxFileSize == 0 {
		cfg.MaxFileSize = 1048576 // 1MB
	}
	if cfg.ExcludePatterns == nil {
		cfg.ExcludePatterns = defaultExcludePatterns()
	}
}

func validate(cfg Config) error {
	if len(cfg.WatchDirs) == 0 {
		return errors.New("watchDirs must not be empty")
	}
	for _, dir := range cfg.WatchDirs {
		info, err := os.Stat(dir)
		if err != nil {
			return fmt.Errorf("watchDir %q: %w", dir, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("watchDir %q is not a directory", dir)
		}
	}
	if cfg.DebounceSec < 1 {
		return errors.New("debounceSec must be >= 1")
	}
	if cfg.Port < 1 || cfg.Port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}
	if cfg.MaxFileSize < 1 {
		return errors.New("maxFileSize must be >= 1")
	}
	if cfg.MaxSnapshots < 0 {
		return errors.New("maxSnapshots must be >= 0")
	}
	if cfg.BasicAuth != nil {
		if cfg.BasicAuth.Username == "" {
			return errors.New("basicAuth.username must not be empty when basicAuth is configured")
		}
		if cfg.BasicAuth.Password == "" {
			return errors.New("basicAuth.password must not be empty when basicAuth is configured")
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
