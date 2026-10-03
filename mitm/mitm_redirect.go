package main

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/elazarl/goproxy"
)

type mitmConfig struct {
	ListenAddr  string            `json:"listen_addr"`
	CACertPath  string            `json:"ca_cert_path"`
	CAKeyPath   string            `json:"ca_key_path"`
	UpstreamURL string            `json:"upstream_url"`
	Hosts       []string          `json:"hosts"`
	Routes      map[string]string `json:"routes"`
}

func loadConfig(path string) (*mitmConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg mitmConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func matchesHost(host string, patterns []string) bool {
	host = strings.ToLower(strings.Split(host, ":")[0])
	for _, p := range patterns {
		p = strings.ToLower(p)
		if host == p || strings.HasSuffix(host, "."+p) {
			return true
		}
	}
	return false
}
func parseUpstream(raw string) *url.URL {
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil
	}
	return u
}
func (c *mitmConfig) upstreamFor(host string) (*url.URL, bool) {
	best := ""
	for pattern := range c.Routes {
		if matchesHost(host, []string{pattern}) && len(pattern) > len(best) {
			best = pattern
		}
	}
	if best != "" {
		if u := parseUpstream(c.Routes[best]); u != nil {
			return u, true
		}
		return nil, false
	}
	if matchesHost(host, c.Hosts) {
		if u := parseUpstream(c.UpstreamURL); u != nil {
			return u, true
		}
	}
	return nil, false
}

func main() {
	webMode := false
	cfgPath := ""
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		switch {
		case arg == "--web":
			webMode = true
		case arg == "--config" && i+1 < len(os.Args):
			i++
			cfgPath = os.Args[i]
		case strings.HasPrefix(arg, "--config="):
			cfgPath = strings.TrimPrefix(arg, "--config=")
		case strings.HasPrefix(arg, "-"):
			log.Fatalf("unknown flag: %s", arg)
		case cfgPath == "":
			cfgPath = arg
		default:
			log.Fatalf("unexpected argument: %s", arg)
		}
	}
	if cfgPath == "" {
		cfgPath = "mitm/config.json"
	}
	cfg, err := loadConfig(cfgPath)
	if err != nil {
		log.Fatalf("failed to load %s: %v", cfgPath, err)
	}
	if webMode {
		if err := runMitmweb(cfgPath, cfg); err != nil {
			log.Fatal(err)
		}
		return
	}

	caCert, err := os.ReadFile(cfg.CACertPath)
	if err != nil {
		log.Fatalf("read ca cert: %v", err)
	}
	caKey, err := os.ReadFile(cfg.CAKeyPath)
	if err != nil {
		log.Fatalf("read ca key: %v", err)
	}
	ca, err := tls.X509KeyPair(caCert, caKey)
	if err != nil {
		log.Fatalf("parse ca keypair: %v", err)
	}
	goproxy.GoproxyCa = ca

	proxy := goproxy.NewProxyHttpServer()
	proxy.Verbose = true
	proxy.Tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}

	condition := goproxy.ReqConditionFunc(func(req *http.Request, ctx *goproxy.ProxyCtx) bool {
		_, ok := cfg.upstreamFor(req.Host)
		return ok
	})

	proxy.OnRequest(condition).HandleConnect(goproxy.AlwaysMitm)

	proxy.OnRequest(condition).
		DoFunc(func(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
			originalHost := req.Host
			up, ok := cfg.upstreamFor(originalHost)
			if !ok {
				return req, nil
			}
			req.URL.Scheme = up.Scheme
			req.URL.Host = up.Host
			req.Host = up.Host
			req.Header.Set("X-Private-Notes-Original-Host", originalHost)
			return req, nil
		})

	log.Printf("private-notes mitm listening on %s, redirecting %v to %s, routes: %v", cfg.ListenAddr, cfg.Hosts, cfg.UpstreamURL, cfg.Routes)
	log.Fatal(http.ListenAndServe(cfg.ListenAddr, proxy))
}

func runMitmweb(cfgPath string, cfg *mitmConfig) error {
	configPath, err := filepath.Abs(cfgPath)
	if err != nil {
		return err
	}
	cert, err := os.ReadFile(cfg.CACertPath)
	if err != nil {
		return fmt.Errorf("read CA certificate: %w", err)
	}
	key, err := os.ReadFile(cfg.CAKeyPath)
	if err != nil {
		return fmt.Errorf("read CA private key: %w", err)
	}
	confDir := filepath.Join(filepath.Dir(cfg.CACertPath), "mitmweb")
	if err := os.MkdirAll(confDir, 0700); err != nil {
		return fmt.Errorf("create mitmweb config directory: %w", err)
	}
	caPath := filepath.Join(confDir, "mitmproxy-ca.pem")
	if err := os.WriteFile(caPath, append(append(cert, '\n'), key...), 0600); err != nil {
		return fmt.Errorf("write mitmweb CA: %w", err)
	}

	listenHost, listenPort := splitListenAddr(cfg.ListenAddr)
	args := []string{
		"--listen-host", listenHost,
		"--listen-port", listenPort,
		"--set", "confdir=" + confDir,
		"--set", "ssl_insecure=true",
		"--web-open-browser",
		"-s", filepath.Join(filepath.Dir(configPath), "redirect_web.py"),
	}
	cmd := exec.Command("mitmweb", args...)
	cmd.Env = append(os.Environ(), "PRIVATE_NOTES_MITM_CONFIG="+configPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	log.Printf("starting mitmweb for redirect proxy on %s; web UI defaults to http://127.0.0.1:8081", cfg.ListenAddr)
	return cmd.Run()
}

func splitListenAddr(addr string) (string, string) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "0.0.0.0", ""
	}
	if host == "" {
		host = "0.0.0.0"
	}
	return host, port
}
