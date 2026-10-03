package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)
const (
	modeCache   = "cache"
	modeForward = "forward"
	modeOffline = "offline"
)

const authEnv = "ASSETBUNDLE_UPSTREAM_AUTHORIZATION"

type UpstreamConfig struct {
	Root string `json:"root"`
	Authorization string `json:"authorization"`
	UnityVersion  string `json:"unity_version"`
	UserAgent     string `json:"user_agent"`
}

type Config struct {
	Listen string `json:"listen"`
	Mode   string `json:"mode"`
	BasePath   string         `json:"base_path"`
	Upstream   UpstreamConfig `json:"upstream"`
	OverlayDir string         `json:"overlay_dir"`
	CacheDir   string         `json:"cache_dir"`
	CatalogFile string `json:"catalog_file"`
	MasterAlias map[string]string `json:"master_alias"`
	AutoHash bool `json:"auto_hash"`
}

func defaultConfig() Config {
	return Config{
		Listen:     "127.0.0.1:5081",
		Mode:       modeCache,
		OverlayDir: "assetbundle_server/data/overlay",
		CacheDir:   "assetbundle_server/data/cache",
		AutoHash:   true,
		Upstream: UpstreamConfig{
			UnityVersion: "6000.3.12f1",
			UserAgent:    "UnityPlayer/6000.3.12f1 (UnityWebRequest/1.0, libcurl/8.10.1-DEV)",
		},
	}
}

func loadConfig(path string) (Config, error) {
	cfg := defaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("%s: %w", path, err)
	}
	if v := strings.TrimSpace(os.Getenv(authEnv)); v != "" {
		cfg.Upstream.Authorization = v
	}
	cfg.BasePath = strings.TrimRight(cfg.BasePath, "/")
	if cfg.BasePath != "" && !strings.HasPrefix(cfg.BasePath, "/") {
		cfg.BasePath = "/" + cfg.BasePath
	}
	return cfg, cfg.validate()
}

func (c Config) validate() error {
	switch c.Mode {
	case modeCache, modeForward, modeOffline:
	default:
		return fmt.Errorf("mode must be cache, forward or offline, got %q", c.Mode)
	}
	if c.Mode != modeOffline && strings.TrimSpace(c.Upstream.Root) == "" {
		return fmt.Errorf("upstream.root is required unless mode is offline")
	}
	if c.OverlayDir == "" || c.CacheDir == "" {
		return fmt.Errorf("overlay_dir and cache_dir must be set")
	}
	for alias, base := range c.MasterAlias {
		if alias == "" || base == "" || alias == base {
			return fmt.Errorf("master_alias entry %q -> %q is invalid", alias, base)
		}
		if _, nested := c.MasterAlias[base]; nested {
			return fmt.Errorf("master_alias %q points at another alias (%q)", alias, base)
		}
		if strings.ContainsAny(alias+base, `/\:`) {
			return fmt.Errorf("master_alias versions must not contain path separators")
		}
	}
	return nil
}