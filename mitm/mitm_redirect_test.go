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