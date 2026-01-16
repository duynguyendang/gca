package meb

import (
	"path/filepath"

	"github.com/duynguyendang/gca/pkg/meb/store"
)

// Open opens a MEBStore at the given path with the provided configuration.
// If no configuration is provided, it uses the default configuration.
func Open(path string, cfg *store.Config) (*MEBStore, error) {
	if cfg == nil {
		cfg = store.DefaultConfig(path)
	} else {
		// Ensure DataDir is set correctly if not provided or different
		if cfg.DataDir == "" {
			cfg.DataDir = path
		}
		// If DictDir is not set, set it to default relative to DataDir
		if cfg.DictDir == "" {
			cfg.DictDir = filepath.Join(cfg.DataDir, "dict")
		}
	}

	return NewMEBStore(cfg)
}
