package config

import (
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"
)

// Config represents the application configuration.
type Config struct {
	Webdav WebdavConfig `toml:"webdav"`
}

// WebdavConfig holds the WebDAV connection details.
type WebdavConfig struct {
	URL      string `toml:"url"`
	User     string `toml:"user"`
	Password string `toml:"password"`
}

// Load reads and parses the TOML configuration file.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse toml: %w", err)
	}

	return &cfg, nil
}
