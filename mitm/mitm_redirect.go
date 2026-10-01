package main

import (
	"crypto/tls"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/elazarl/goproxy"
)

type mitmConfig struct {
	ListenAddr  string   `json:"listen_addr"`
	CACertPath  string   `json:"ca_cert_path"`
	CAKeyPath   string   `json:"ca_key_path"`
	UpstreamURL string   `json:"upstream_url"`
	Hosts       []string `json:"hosts"`
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

func main() {
	cfgPath := "config.json"
	if len(os.Args) > 1 {
		cfgPath = os.Args[1]
	}
	cfg, err := loadConfig(cfgPath)
	if err != nil {
		log.Fatalf("failed to load %s: %v", cfgPath, err)
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
		return matchesHost(req.Host, cfg.Hosts)
	})

	proxy.OnRequest(condition).HandleConnect(goproxy.AlwaysMitm)

	proxy.OnRequest(condition).
		DoFunc(func(req *http.Request, ctx *goproxy.ProxyCtx) (*http.Request, *http.Response) {
			originalHost := req.Host
			req.URL.Scheme = "https"
			req.URL.Host = strings.TrimPrefix(cfg.UpstreamURL, "https://")
			req.Host = req.URL.Host
			req.Header.Set("X-Private-Dori-Original-Host", originalHost)
			return req, nil
		})

	log.Printf("private-notes mitm listening on %s, redirecting %v to %s", cfg.ListenAddr, cfg.Hosts, cfg.UpstreamURL)
	log.Fatal(http.ListenAndServe(cfg.ListenAddr, proxy))
}