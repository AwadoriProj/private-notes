package main

import "testing"

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