package main

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/awadoriproj/awadori-assets/catalog"
	"github.com/awadoriproj/awadori-assets/crypto"
	"github.com/awadoriproj/awadori-assets/masterdata"
)

type Server struct {
	cfg    Config
	client *http.Client
	flight flightGroup

	provMu   sync.Mutex
	provs    map[string]string
	provMod  time.Time
	provNext time.Time

	hashMu   sync.Mutex
	hashMemo map[string]hashEntry
}

type hashEntry struct {
	size int64
	mod  time.Time
	sum  string
}

func NewServer(cfg Config) *Server {
	return &Server{cfg: cfg, client: newHTTPClient(), hashMemo: map[string]hashEntry{}}
}

type content struct {
	r     io.ReadSeeker
	mod   time.Time
	layer string
	close func()
}

func (c *content) Close() {
	if c.close != nil {
		c.close()
	}
}

func (s *Server) normalize(urlPath string) (string, bool) {
	p := path.Clean("/" + urlPath)
	if base := s.cfg.BasePath; base != "" && strings.HasPrefix(p, base+"/") {
		p = p[len(base):]
	}
	p = strings.TrimPrefix(p, "/")
	parts := strings.Split(p, "/")
	if len(parts) < 3 || (parts[0] != "asset" && parts[0] != "master") {
		return "", false
	}
	if parts[0] == "master" && len(parts) != 3 {
		return "", false
	}
	for _, seg := range parts {
		if seg == "" || strings.HasPrefix(seg, ".") || strings.ContainsAny(seg, `\:`) {
			return "", false
		}
	}
	if strings.HasSuffix(parts[2], ".part") {
		return "", false
	}
	return p, true
}

func (s *Server) overlayPath(rel string) string {
	return filepath.Join(s.cfg.OverlayDir, filepath.FromSlash(rel))
}
func (s *Server) cachePath(rel string) string {
	return filepath.Join(s.cfg.CacheDir, filepath.FromSlash(rel))
}

func (s *Server) providerFor(name string) string {
	s.provMu.Lock()
	defer s.provMu.Unlock()
	if s.cfg.CatalogFile != "" && time.Now().After(s.provNext) {
		s.provNext = time.Now().Add(2 * time.Second)
		if st, err := os.Stat(s.cfg.CatalogFile); err == nil && !st.ModTime().Equal(s.provMod) {
			if data, err := os.ReadFile(s.cfg.CatalogFile); err == nil {
				if cat, err := catalog.Parse(data); err == nil {
					m := map[string]string{}
					for _, l := range cat.Locs {
						if l.Opts != nil && strings.Contains(l.Internal, catalog.AssetMarker) {
							m[catalog.BundleName(l)] = l.Provider
						}
					}
					s.provs, s.provMod = m, st.ModTime()
					log.Printf("catalog %s loaded, %d bundle providers known", s.cfg.CatalogFile, len(m))
				} else {
					log.Printf("catalog %s unreadable: %v", s.cfg.CatalogFile, err)
				}
			}
		}
	}
	if p, ok := s.provs[name]; ok {
		return p
	}
	return crypto.Provider
}

func openFile(p, layer string) (*content, error) {
	f, err := os.Open(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errNotFound
		}
		return nil, err
	}
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		f.Close()
		return nil, errNotFound
	}
	return &content{r: f, mod: st.ModTime(), layer: layer, close: func() { f.Close() }}, nil
}

func (s *Server) openOverlay(rel string) (*content, error) {
	p := s.overlayPath(rel)
	name := path.Base(rel)
	if strings.HasPrefix(rel, "asset/") && strings.HasSuffix(strings.ToLower(name), ".bundle") {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			ef, err := crypto.OpenEncrypted(p, name, s.providerFor(name))
			if err == nil {
				return &content{r: io.NewSectionReader(ef, 0, ef.Size()), mod: st.ModTime(),
					layer: "overlay+encrypt", close: func() { ef.Close() }}, nil
			}
			if !errors.Is(err, crypto.ErrAlreadyEncrypted) {
				return nil, err
			}
		}
	}
	return openFile(p, "overlay")
}

func (s *Server) autoHash(rel string) (*content, error) {
	if !s.cfg.AutoHash || !strings.HasPrefix(rel, "asset/") || !strings.HasSuffix(rel, ".hash") {
		return nil, errNotFound
	}
	binPath := s.overlayPath(strings.TrimSuffix(rel, ".hash") + ".bin")
	st, err := os.Stat(binPath)
	if err != nil || st.IsDir() {
		return nil, errNotFound
	}
	s.hashMu.Lock()
	defer s.hashMu.Unlock()
	e, ok := s.hashMemo[binPath]
	if !ok || e.size != st.Size() || !e.mod.Equal(st.ModTime()) {
		data, err := os.ReadFile(binPath)
		if err != nil {
			return nil, err
		}
		sum := md5.Sum(data)
		e = hashEntry{size: st.Size(), mod: st.ModTime(), sum: hex.EncodeToString(sum[:])}
		s.hashMemo[binPath] = e
	}
	return &content{r: bytes.NewReader([]byte(e.sum)), mod: e.mod, layer: "overlay+autohash"}, nil
}

func (s *Server) openLayers(ctx context.Context, rel string) (*content, error) {
	if c, err := s.openOverlay(rel); !errors.Is(err, errNotFound) {
		return c, err
	}
	if c, err := s.autoHash(rel); !errors.Is(err, errNotFound) {
		return c, err
	}
	if c, err := openFile(s.cachePath(rel), "cache"); !errors.Is(err, errNotFound) {
		return c, err
	}
	if s.cfg.Mode == modeCache {
		p, err := s.fetchToCache(ctx, rel)
		if err != nil {
			return nil, err
		}
		c, err := openFile(p, "upstream->cache")
		return c, err
	}
	return nil, errNotFound
}

func (s *Server) readLayers(ctx context.Context, rel string) ([]byte, error) {
	c, err := s.openLayers(ctx, rel)
	if err == nil {
		defer c.Close()
		return io.ReadAll(c.r)
	}
	if errors.Is(err, errNotFound) && s.cfg.Mode == modeForward {
		return s.fetchBytes(ctx, rel)
	}
	return nil, err
}

func (s *Server) open(ctx context.Context, rel string) (*content, error) {
	parts := strings.Split(rel, "/")
	if parts[0] == "master" {
		if base, ok := s.cfg.MasterAlias[parts[1]]; ok {
			return s.openAliased(ctx, parts[1], base, parts[2])
		}
	}
	return s.openLayers(ctx, rel)
}

func (s *Server) openAliased(ctx context.Context, alias, base, name string) (*content, error) {
	own := "master/" + alias + "/" + name
	if c, err := openFile(s.overlayPath(own), "overlay"); !errors.Is(err, errNotFound) {
		return c, err
	}
	baseRel := "master/" + base + "/" + name
	if name != masterdata.ManifestName {
		return s.openLayers(ctx, baseRel)
	}
	raw, err := s.readLayers(ctx, baseRel)
	if err != nil {
		return nil, err
	}
	var m masterdata.Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, errors.New("base master manifest is not valid JSON: " + err.Error())
	}
	m.Version = alias
	dir := s.overlayPath("master/" + alias + "/x")
	dir = filepath.Dir(dir)
	entries, _ := os.ReadDir(dir)
	index := map[string]int{}
	for i, f := range m.Files {
		index[f.Name] = i
	}
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".bin") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, err
		}
		sum := sha256.Sum256(data)
		f := masterdata.ManifestFile{Name: e.Name(), Hash: hex.EncodeToString(sum[:]), Size: int64(len(data))}
		if i, ok := index[e.Name()]; ok {
			m.Files[i] = f
		} else {
			m.Files = append(m.Files, f)
		}
	}
	out, err := json.MarshalIndent(&m, "", "  ")
	if err != nil {
		return nil, err
	}
	return &content{r: bytes.NewReader(out), mod: time.Now(), layer: "master-alias"}, nil
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
	layer := "-"
	rel, ok := s.normalize(r.URL.Path)
	defer func() {
		if ok {
			log.Printf("%s %s -> %d [%s]", r.Method, rel, rec.status, layer)
		}
	}()
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(rec, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !ok {
		log.Printf("refused path %q", r.URL.Path)
		http.NotFound(rec, r)
		return
	}
	c, err := s.open(r.Context(), rel)
	switch {
	case err == nil:
		defer c.Close()
		layer = c.layer
		name := path.Base(rel)
		switch strings.ToLower(path.Ext(name)) {
		case ".bundle", ".bin":
			rec.Header().Set("Content-Type", "application/octet-stream")
		case ".hash":
			rec.Header().Set("Content-Type", "text/plain")
		}
		http.ServeContent(rec, r, name, c.mod, c.r)
	case errors.Is(err, errNotFound) && s.cfg.Mode == modeForward:
		layer = "forward"
		if _, ferr := s.forward(rec, r, rel); ferr != nil {
			s.fail(rec, rel, ferr, &layer)
		}
	default:
		s.fail(rec, rel, err, &layer)
	}
}

func (s *Server) fail(w http.ResponseWriter, rel string, err error, layer *string) {
	switch {
	case errors.Is(err, errNotFound):
		*layer = "missing"
		log.Printf("WARN not found in any layer: %s (mode %s)", rel, s.cfg.Mode)
		http.Error(w, "not found", http.StatusNotFound)
	case errors.Is(err, errUpstream):
		*layer = "upstream-error"
		log.Printf("WARN %v", err)
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
	default:
		*layer = "error"
		log.Printf("ERROR %s: %v", rel, err)
		http.Error(w, "internal error", http.StatusInternalServerError)
	}
}
