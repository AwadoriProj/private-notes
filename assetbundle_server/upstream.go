package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	errNotFound = errors.New("not found")
	errUpstream = errors.New("upstream error")
)

func newHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			ResponseHeaderTimeout: 30 * time.Second,
			MaxIdleConnsPerHost:   8,
			IdleConnTimeout:       60 * time.Second,
		},
	}
}

func (s *Server) upstreamRequest(ctx context.Context, method, rel string) (*http.Request, error) {
	u := strings.TrimRight(s.cfg.Upstream.Root, "/") + "/" + rel
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "*/*")
	if v := s.cfg.Upstream.UserAgent; v != "" {
		req.Header.Set("User-Agent", v)
	}
	if v := s.cfg.Upstream.UnityVersion; v != "" {
		req.Header.Set("X-Unity-Version", v)
	}
	if v := s.cfg.Upstream.Authorization; v != "" {
		req.Header.Set("Authorization", v)
	}
	return req, nil
}

func checkUpstreamStatus(rel string, code int) error {
	switch {
	case code == http.StatusOK || code == http.StatusPartialContent:
		return nil
	case code == http.StatusNotFound:
		return errNotFound
	case code == http.StatusUnauthorized || code == http.StatusForbidden:
		return fmt.Errorf("%w: %s answered %d, check upstream.authorization", errUpstream, rel, code)
	default:
		return fmt.Errorf("%w: %s answered %d", errUpstream, rel, code)
	}
}

type flightGroup struct {
	mu    sync.Mutex
	calls map[string]*flightCall
}

type flightCall struct {
	wg  sync.WaitGroup
	err error
}

func (g *flightGroup) Do(key string, fn func() error) error {
	g.mu.Lock()
	if g.calls == nil {
		g.calls = map[string]*flightCall{}
	}
	if c, ok := g.calls[key]; ok {
		g.mu.Unlock()
		c.wg.Wait()
		return c.err
	}
	c := &flightCall{}
	c.wg.Add(1)
	g.calls[key] = c
	g.mu.Unlock()

	c.err = fn()
	c.wg.Done()

	g.mu.Lock()
	delete(g.calls, key)
	g.mu.Unlock()
	return c.err
}

func (s *Server) fetchToCache(ctx context.Context, rel string) (string, error) {
	dst := s.cachePath(rel)
	err := s.flight.Do(rel, func() error {
		if _, err := os.Stat(dst); err == nil {
			return nil
		}
		dctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Minute)
		defer cancel()
		req, err := s.upstreamRequest(dctx, http.MethodGet, rel)
		if err != nil {
			return err
		}
		resp, err := s.client.Do(req)
		if err != nil {
			return fmt.Errorf("%w: %v", errUpstream, err)
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusPartialContent {
			return fmt.Errorf("%w: unexpected 206 for %s", errUpstream, rel)
		}
		if err := checkUpstreamStatus(rel, resp.StatusCode); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		tmp := dst + ".part"
		out, err := os.Create(tmp)
		if err != nil {
			return err
		}
		n, err := io.Copy(out, resp.Body)
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err == nil && !resp.Uncompressed && resp.ContentLength >= 0 && n != resp.ContentLength {
			err = fmt.Errorf("short download: got %d of %d bytes", n, resp.ContentLength)
		}
		if err != nil {
			os.Remove(tmp)
			return fmt.Errorf("%w: %s: %v", errUpstream, rel, err)
		}
		return os.Rename(tmp, dst)
	})
	return dst, err
}

func (s *Server) fetchBytes(ctx context.Context, rel string) ([]byte, error) {
	req, err := s.upstreamRequest(ctx, http.MethodGet, rel)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errUpstream, err)
	}
	defer resp.Body.Close()
	if err := checkUpstreamStatus(rel, resp.StatusCode); err != nil {
		return nil, err
	}
	return io.ReadAll(io.LimitReader(resp.Body, 64<<20))
}

func (s *Server) forward(w http.ResponseWriter, r *http.Request, rel string) (int, error) {
	req, err := s.upstreamRequest(r.Context(), r.Method, rel)
	if err != nil {
		return 0, err
	}
	for _, h := range []string{"Range", "If-None-Match", "If-Modified-Since", "If-Range"} {
		if v := r.Header.Get(h); v != "" {
			req.Header.Set(h, v)
		}
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("%w: %v", errUpstream, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotModified && resp.StatusCode != http.StatusRequestedRangeNotSatisfiable {
		if err := checkUpstreamStatus(rel, resp.StatusCode); err != nil {
			return 0, err
		}
	}
	hdr := w.Header()
	for _, h := range []string{"Content-Type", "Content-Range", "Accept-Ranges", "Etag", "Last-Modified"} {
		if v := resp.Header.Get(h); v != "" {
			hdr.Set(h, v)
		}
	}
	if !resp.Uncompressed && resp.ContentLength >= 0 {
		hdr.Set("Content-Length", fmt.Sprint(resp.ContentLength))
	}
	w.WriteHeader(resp.StatusCode)
	if r.Method != http.MethodHead {
		io.Copy(w, resp.Body)
	}
	return resp.StatusCode, nil
}