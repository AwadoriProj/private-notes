package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode"
)

func main() {
	cfgPath := flag.String("config", "assetbundle_server/config.json", "path of the JSON config")
	probe := flag.String("probe", "", "fetch this path (e.g. asset/Android/catalog_1.0.0.201_en.hash) from upstream with the configured credentials, print the result and exit")
	flag.Parse()

	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	srv := NewServer(cfg)

	if *probe != "" {
		os.Exit(runProbe(srv, *probe))
	}

	if cfg.Mode != modeOffline && cfg.Upstream.Authorization == "" {
		log.Printf("WARN no upstream authorization set: every request in the captures carried an Authorization header (set %s)", authEnv)
	}
	log.Printf("assetbundle_server listening on %s, mode=%s, overlay=%s, cache=%s, aliases=%d",
		cfg.Listen, cfg.Mode, cfg.OverlayDir, cfg.CacheDir, len(cfg.MasterAlias))
	hs := &http.Server{Addr: cfg.Listen, Handler: srv, ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(hs.ListenAndServe())
}
func runProbe(s *Server, rel string) int {
	rel = strings.TrimPrefix(rel, "/")
	req, err := s.upstreamRequest(context.Background(), http.MethodGet, rel)
	if err != nil {
		fmt.Println("bad probe path:", err)
		return 2
	}
	fmt.Printf("GET %s (authorization sent: %v)\n", req.URL.String(), req.Header.Get("Authorization") != "")
	resp, err := s.client.Do(req)
	if err != nil {
		fmt.Println("request failed:", err)
		return 1
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	fmt.Printf("status %d, %d bytes read, content-type %q\n", resp.StatusCode, len(body), resp.Header.Get("Content-Type"))
	if len(body) > 0 && len(body) <= 128 && strings.IndexFunc(string(body), func(r rune) bool { return !unicode.IsPrint(r) && !unicode.IsSpace(r) }) < 0 {
		fmt.Printf("body: %s\n", body)
	}
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}