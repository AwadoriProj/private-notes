package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestMatchesHost(t *testing.T) {
	patterns := []string{"biligame.net", "bilibiligame.net", "gamerfusiontech.com"}

	cases := map[string]bool{
		"l11-sdk-login-intl.biligame.net:443":               true,
		"l14-prod-sg-patch-sirius.bilibiligame.net":         true,
		"l14-prod-hk-all-gs-sirius.gamerfusiontech.com:443": true,
		"example.com":           false,
		"notbiligame.net":       false,
		"biligame.net.evil.com": false,
	}

	for host, want := range cases {
		if got := matchesHost(host, patterns); got != want {
			t.Errorf("matchesHost(%q) = %v, want %v", host, got, want)
		}
	}
}

func TestUpstreamFor(t *testing.T) {
	cfg := &mitmConfig{
		UpstreamURL: "https://game.local:9443",
		Hosts:       []string{"biligame.net", "bilibiligame.net"},
		Routes: map[string]string{
			"l14-prod-sg-patch-sirius.bilibiligame.net": "http://127.0.0.1:5081",
			"bilibiligame.net":                          "https://other.local:1234",
		},
	}
	cases := []struct {
		host   string
		want   string
		wantOK bool
	}{
		{"l14-prod-sg-patch-sirius.bilibiligame.net:443", "http://127.0.0.1:5081", true},
		{"cdn.bilibiligame.net", "https://other.local:1234", true},
		{"l11-sdk-login-intl.biligame.net", "https://game.local:9443", true},
		{"example.com", "", false},
		{"biligame.net.evil.com", "", false},
	}
	for _, c := range cases {
		u, ok := cfg.upstreamFor(c.host)
		if ok != c.wantOK || (ok && u.String() != c.want) {
			t.Errorf("upstreamFor(%q) = %v, %v; want %q, %v", c.host, u, ok, c.want, c.wantOK)
		}
	}
	legacy := &mitmConfig{UpstreamURL: "game.local:9443", Hosts: []string{"biligame.net"}}
	if u, ok := legacy.upstreamFor("x.biligame.net"); !ok || u.Scheme != "https" || u.Host != "game.local:9443" {
		t.Errorf("legacy config resolved to %v, %v", u, ok)
	}
	broken := &mitmConfig{UpstreamURL: "", Hosts: []string{"biligame.net"}}
	if _, ok := broken.upstreamFor("x.biligame.net"); ok {
		t.Error("an empty upstream_url must not match")
	}
}

func TestMitmwebArgsRegular(t *testing.T) {
	cfg := &mitmConfig{ListenAddr: ":8443", WireGuardPort: defaultWireGuardPort}
	got := mitmwebArgs(cfg, "conf", "redirect_web.py")
	want := []string{
		"--listen-port", "8443",
		"--set", "confdir=conf",
		"--set", "ssl_insecure=true",
		"--web-open-browser",
		"-s", "redirect_web.py",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("args = %v, want %v", got, want)
	}
}

func TestMitmwebArgsWireGuard(t *testing.T) {
	cfg := &mitmConfig{ListenAddr: ":8443", WireGuard: true, WireGuardPort: 51821}
	got := mitmwebArgs(cfg, "conf", "redirect_web.py")
	want := []string{
		"--mode", "regular@8443",
		"--mode", "wireguard@51821",
		"--set", "confdir=conf",
		"--set", "ssl_insecure=true",
		"--web-open-browser",
		"-s", "redirect_web.py",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("args = %v, want %v", got, want)
	}
	bare := &mitmConfig{ListenAddr: "bad", WireGuard: true, WireGuardPort: defaultWireGuardPort}
	args := mitmwebArgs(bare, "conf", "redirect_web.py")
	if args[1] != "regular" || args[3] != "wireguard@51820" {
		t.Errorf("fallback args = %v", args)
	}
}

func TestLoadConfigWireGuard(t *testing.T) {
	dir := t.TempDir()
	write := func(body string) string {
		path := filepath.Join(dir, "config.json")
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	cfg, err := loadConfig(write(`{"listen_addr": ":8443"}`))
	if err != nil || cfg.WireGuard || cfg.WireGuardPort != defaultWireGuardPort {
		t.Fatalf("defaults: %+v %v", cfg, err)
	}
	cfg, err = loadConfig(write(`{"wireguard": true, "wireguard_port": 51999}`))
	if err != nil || !cfg.WireGuard || cfg.WireGuardPort != 51999 {
		t.Fatalf("explicit: %+v %v", cfg, err)
	}
	for _, bad := range []string{`{"wireguard_port": -1}`, `{"wireguard_port": 70000}`} {
		if _, err := loadConfig(write(bad)); err == nil {
			t.Errorf("%s must be rejected", bad)
		}
	}
}


func TestMitmwebArgsSpecificHost(t *testing.T) {
	cfg := &mitmConfig{ListenAddr: "192.168.1.10:8443", WireGuard: true, WireGuardPort: defaultWireGuardPort}
	got := mitmwebArgs(cfg, "conf", "redirect_web.py")
	if len(got) < 2 || got[0] != "--listen-host" || got[1] != "192.168.1.10" {
		t.Errorf("a specific listen host must be passed through: %v", got)
	}
}
