// Package config handles on-disk state under ~/.leroymerlin: the user-authored
// config.toml (defaults + optional cookie) and the machine-managed cookie cache.
// LEROYMERLIN_CONFIG_DIR overrides the directory.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const configFile = "config.toml"

// Dir is ~/.leroymerlin (or $LEROYMERLIN_CONFIG_DIR).
func Dir() string {
	if d := os.Getenv("LEROYMERLIN_CONFIG_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ".leroymerlin"
	}
	return filepath.Join(home, ".leroymerlin")
}

// Config mirrors ~/.leroymerlin/config.toml.
type Config struct {
	Auth struct {
		Cookie string `toml:"cookie"` // browser cookie (DataDome clearance) for challenged reads
	} `toml:"auth"`
	Defaults struct {
		Lang string `toml:"lang"` // "es" (default) or "ca"
	} `toml:"defaults"`
}

// LoadConfig reads config.toml. A missing file is not an error (empty config);
// any other stat error is returned.
func LoadConfig() (Config, error) {
	var c Config
	p := filepath.Join(Dir(), configFile)
	if _, err := os.Stat(p); err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return c, fmt.Errorf("stat config %s: %w", p, err)
	}
	_, err := toml.DecodeFile(p, &c)
	return c, err
}

// ensureDir creates the config dir (0700; it may hold a session cookie) if absent.
func ensureDir() error {
	return os.MkdirAll(Dir(), 0o700)
}

// Load reads a JSON state file (e.g. the cookie cache) from the config dir.
func Load(name string, v any) error {
	b, err := os.ReadFile(filepath.Join(Dir(), name))
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// Save writes v as pretty JSON to name in the config dir, 0600. The write is
// atomic (temp file + rename) so a crash mid-write cannot corrupt the cache.
func Save(name string, v any) error {
	if err := ensureDir(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	dir := Dir()
	tmp, err := os.CreateTemp(dir, name+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, filepath.Join(dir, name))
}
