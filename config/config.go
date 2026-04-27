package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	DefaultSprintLengthDays = 7
	appDirName              = ".lira"
	configFileName          = "config.json"
	dbFileName              = "lira.db"
)

// Config holds all user-configurable settings for Lira.
type Config struct {
	SprintLengthDays int    `json:"sprint_length_days"`
	DBPath           string `json:"db_path"`
}

// Load reads the config file from the default location (~/.lira/config.json).
// If the file does not exist, a default config is returned and saved to disk.
func Load() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		cfg := defaults()
		if err := save(cfg, path); err != nil {
			return nil, fmt.Errorf("write default config: %w", err)
		}
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	cfg.applyDefaults()
	return &cfg, nil
}

// Save writes the current config back to disk.
func (c *Config) Save() error {
	path, err := configPath()
	if err != nil {
		return err
	}
	return save(c, path)
}

// AppDir returns the absolute path to the ~/.lira directory, creating it if needed.
func AppDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	dir := filepath.Join(home, appDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create app dir: %w", err)
	}
	return dir, nil
}

// --- internal helpers ---

func defaults() *Config {
	dir, _ := AppDir() // error surfaced separately on real startup
	return &Config{
		SprintLengthDays: DefaultSprintLengthDays,
		DBPath:           filepath.Join(dir, dbFileName),
	}
}

// applyDefaults fills in zero values after unmarshalling, so missing keys in
// an older config file do not break the app.
func (c *Config) applyDefaults() {
	if c.SprintLengthDays <= 0 {
		c.SprintLengthDays = DefaultSprintLengthDays
	}
	if c.DBPath == "" {
		dir, _ := AppDir()
		c.DBPath = filepath.Join(dir, dbFileName)
	}
}

func configPath() (string, error) {
	dir, err := AppDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configFileName), nil
}

func save(cfg *Config, path string) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
