package main

import (
	"bytes"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/awadoriproj/awadori-assets/crypto"
	"github.com/awadoriproj/awadori-assets/masterdata"
)

var testModTime = time.Unix(1790000000, 0)

const (
	testBase = "/prod/en_test"
	testAuth = "Basic dGVzdDp0ZXN0"
)

type fakeUpstream struct {
	*httptest.Server
	hits  atomic.Int64
	files map[string][]byte
}

func newFakeUpstream(t *testing.T, files map[string][]byte) *fakeUpstream {
	t.Helper()
	f := &fakeUpstream{files: files}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.hits.Add(1)
		if r.Header.Get("Authorization") != testAuth || r.Header.Get("X-Unity-Version") == "" {
			http.Error(w, "denied", http.StatusUnauthorized)
			return
		}
		data, ok := f.files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		http.ServeContent(w, r, "x", testModTime, bytes.NewReader(data))
	}))
	t.Cleanup(f.Close)
	return f
}

func newTestServer(t *testing.T, up *fakeUpstream, mutate func(*Config)) (*Server, *httptest.Server) {
	t.Helper()
	cfg := defaultConfig()
	cfg.OverlayDir = filepath.Join(t.TempDir(), "overlay")
	cfg.CacheDir = filepath.Join(t.TempDir(), "cache")
	cfg.BasePath = testBase
	if up != nil {
		cfg.Upstream.Root = up.URL + testBase
		cfg.Upstream.Authorization = testAuth
	}
	if mutate != nil {
		mutate(&cfg)
	}
	if err := cfg.validate(); err != nil {
		t.Fatal(err)
	}
	s := NewServer(cfg)
	ts := httptest.NewServer(s)
	t.Cleanup(ts.Close)
	return s, ts
}

func get(t *testing.T, url string, hdr ...string) (int, []byte, http.Header) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url, nil)
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body, resp.Header
}

func writeFile(t *testing.T, p string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
}
func fakeBundle(n int) []byte {
	b := make([]byte, n)
	copy(b, crypto.Magic)
	for i := len(crypto.Magic); i < n; i++ {
		b[i] = byte(i * 7)
	}
	return b
}

func TestCacheModeFetchesOnceAndServesRanges(t *testing.T) {
	payload := fakeBundle(40000)
	up := newFakeUpstream(t, map[string][]byte{testBase + "/asset/Android/a.bundle": payload})
	_, ts := newTestServer(t, up, nil)

	for i := 0; i < 3; i++ {
		code, body, _ := get(t, ts.URL+testBase+"/asset/Android/a.bundle")
		if code != 200 || !bytes.Equal(body, payload) {
			t.Fatalf("request %d: status %d, %d bytes", i, code, len(body))
		}
	}
	if up.hits.Load() != 1 {
		t.Fatalf("upstream was hit %d times, want 1", up.hits.Load())
	}
	code, body, hdr := get(t, ts.URL+testBase+"/asset/Android/a.bundle", "Range", "bytes=100-199")
	if code != http.StatusPartialContent || !bytes.Equal(body, payload[100:200]) || hdr.Get("Content-Range") == "" {
		t.Fatalf("range request: status %d, %d bytes", code, len(body))
	}
	if code, _, _ := get(t, ts.URL+"/asset/Android/a.bundle"); code != 200 {
		t.Fatalf("unprefixed path: status %d", code)
	}
}

func TestUpstreamAuthFailureIsBadGateway(t *testing.T) {
	up := newFakeUpstream(t, map[string][]byte{testBase + "/asset/Android/a.bundle": fakeBundle(100)})
	_, ts := newTestServer(t, up, func(c *Config) { c.Upstream.Authorization = "Basic wrong" })
	if code, _, _ := get(t, ts.URL+testBase+"/asset/Android/a.bundle"); code != http.StatusBadGateway {
		t.Fatalf("status %d, want 502", code)
	}
	if code, _, _ := get(t, ts.URL+testBase+"/asset/Android/missing.bundle"); code != http.StatusBadGateway {
		t.Fatalf("status %d, want 502", code)
	}
}

func TestMissingFileIs404AndOfflineNeverCallsUpstream(t *testing.T) {
	up := newFakeUpstream(t, map[string][]byte{})
	_, ts := newTestServer(t, up, nil)
	if code, _, _ := get(t, ts.URL+testBase+"/asset/Android/nope.bundle"); code != 404 {
		t.Fatalf("status %d, want 404", code)
	}
	up2 := newFakeUpstream(t, map[string][]byte{testBase + "/asset/Android/a.bundle": fakeBundle(100)})
	_, ts2 := newTestServer(t, up2, func(c *Config) { c.Mode = modeOffline })
	if code, _, _ := get(t, ts2.URL+testBase+"/asset/Android/a.bundle"); code != 404 {
		t.Fatalf("offline status %d, want 404", code)
	}
	if up2.hits.Load() != 0 {
		t.Fatal("offline mode contacted upstream")
	}
}

func TestForwardModeStreamsWithoutStoring(t *testing.T) {
	payload := fakeBundle(5000)
	up := newFakeUpstream(t, map[string][]byte{testBase + "/asset/Android/a.bundle": payload})
	s, ts := newTestServer(t, up, func(c *Config) { c.Mode = modeForward })
	code, body, _ := get(t, ts.URL+testBase+"/asset/Android/a.bundle")
	if code != 200 || !bytes.Equal(body, payload) {
		t.Fatalf("status %d, %d bytes", code, len(body))
	}
	code, body, _ = get(t, ts.URL+testBase+"/asset/Android/a.bundle", "Range", "bytes=10-19")
	if code != http.StatusPartialContent || !bytes.Equal(body, payload[10:20]) {
		t.Fatalf("forwarded range: status %d, %d bytes", code, len(body))
	}
	if _, err := os.Stat(s.cachePath("asset/Android/a.bundle")); err == nil {
		t.Fatal("forward mode stored a file in the cache")
	}
}

func TestOverlayEncryptsDecryptedBundlesOnTheFly(t *testing.T) {
	up := newFakeUpstream(t, map[string][]byte{testBase + "/asset/Android/a.bundle": []byte("upstream copy")})
	s, ts := newTestServer(t, up, nil)
	plain := fakeBundle(30000)
	writeFile(t, s.overlayPath("asset/Android/a.bundle"), plain)

	code, body, _ := get(t, ts.URL+testBase+"/asset/Android/a.bundle")
	if code != 200 || len(body) != len(plain) {
		t.Fatalf("status %d, %d bytes", code, len(body))
	}
	if bytes.Equal(body[:64], plain[:64]) {
		t.Fatal("overlay bundle was served decrypted")
	}
	if !bytes.Equal(body[crypto.Window:], plain[crypto.Window:]) {
		t.Fatal("bytes after the encrypted window changed")
	}
	crypto.Decrypt(body, "a.bundle")
	if !bytes.Equal(body, plain) {
		t.Fatal("decrypting the response does not give the original bundle")
	}
	if up.hits.Load() != 0 {
		t.Fatal("overlay file did not take precedence over upstream")
	}
	_, part, _ := get(t, ts.URL+testBase+"/asset/Android/a.bundle", "Range", "bytes=16380-16390")
	_, whole, _ := get(t, ts.URL+testBase+"/asset/Android/a.bundle")
	if !bytes.Equal(part, whole[16380:16391]) {
		t.Fatal("range across the window boundary differs from the full response")
	}
}

func TestOverlayLeavesAlreadyEncryptedAndPlainProviderFilesAlone(t *testing.T) {
	s, ts := newTestServer(t, nil, func(c *Config) { c.Mode = modeOffline })
	enc := fakeBundle(20000)
	crypto.Encrypt(enc, "pre.bundle")
	writeFile(t, s.overlayPath("asset/Android/pre.bundle"), enc)
	if _, body, _ := get(t, ts.URL+testBase+"/asset/Android/pre.bundle"); !bytes.Equal(body, enc) {
		t.Fatal("an already encrypted overlay file was changed")
	}
	s.provs = map[string]string{"plain.bundle": "UnityEngine.ResourceManagement.ResourceProviders.AssetBundleProvider"}
	plain := fakeBundle(20000)
	writeFile(t, s.overlayPath("asset/Android/plain.bundle"), plain)
	if _, body, _ := get(t, ts.URL+testBase+"/asset/Android/plain.bundle"); !bytes.Equal(body, plain) {
		t.Fatal("a bundle with a non encrypting provider was encrypted")
	}
}

func TestRefusedPaths(t *testing.T) {
	up := newFakeUpstream(t, map[string][]byte{"/etc/passwd": []byte("x")})
	_, ts := newTestServer(t, up, nil)
	for _, p := range []string{
		"/", "/asset/", "/asset/Android", "/other/Android/a.bundle",
		"/asset/../../etc/passwd", "/asset/Android/.hidden",
		"/asset/Android/a.bundle.part", "/asset/Android/c:evil", "/asset/Android/%2e%2e%2fx",
	} {
		if code, _, _ := get(t, ts.URL+p); code != 404 {
			t.Errorf("%s: status %d, want 404", p, code)
		}
	}
	if up.hits.Load() != 0 {
		t.Fatalf("refused paths reached upstream %d times", up.hits.Load())
	}
	req, _ := http.NewRequest(http.MethodPost, ts.URL+testBase+"/asset/Android/a.bundle", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST status %d", resp.StatusCode)
	}
}

func TestAutoHashFollowsOverlayCatalog(t *testing.T) {
	up := newFakeUpstream(t, map[string][]byte{testBase + "/asset/Android/catalog_1_en.hash": []byte("0123456789abcdef0123456789abcdef")})
	s, ts := newTestServer(t, up, nil)
	hashURL := ts.URL + testBase + "/asset/Android/catalog_1_en.hash"

	if _, body, _ := get(t, hashURL); string(body) != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("without an overlay catalog the upstream hash must pass through, got %q", body)
	}
	cat1 := []byte("catalog one")
	writeFile(t, s.overlayPath("asset/Android/catalog_1_en.bin"), cat1)
	sum := md5.Sum(cat1)
	want := hex.EncodeToString(sum[:])
	if _, body, _ := get(t, hashURL); string(body) != want || len(body) != 32 {
		t.Fatalf("hash %q, want %q", body, want)
	}
	cat2 := []byte("catalog two, longer")
	writeFile(t, s.overlayPath("asset/Android/catalog_1_en.bin"), cat2)
	sum = md5.Sum(cat2)
	if _, body, _ := get(t, hashURL); string(body) != hex.EncodeToString(sum[:]) {
		t.Fatal("hash did not change after the catalog changed")
	}
	writeFile(t, s.overlayPath("asset/Android/catalog_1_en.hash"), []byte("explicit"))
	if _, body, _ := get(t, hashURL); string(body) != "explicit" {
		t.Fatalf("explicit hash ignored: %q", body)
	}
}

func sha(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }

func TestMasterAliasMergesOverlayIntoBaseManifest(t *testing.T) {
	foo, bar := []byte("foo original"), []byte("bar original")
	baseManifest, _ := json.Marshal(masterdata.Manifest{Version: "base", Files: []masterdata.ManifestFile{
		{Name: "Foo.bin", Hash: sha(foo), Size: int64(len(foo))},
		{Name: "Bar.bin", Hash: sha(bar), Size: int64(len(bar))},
	}})
	up := newFakeUpstream(t, map[string][]byte{
		testBase + "/master/base/MasterManifest.json": baseManifest,
		testBase + "/master/base/Foo.bin":             foo,
		testBase + "/master/base/Bar.bin":             bar,
	})
	s, ts := newTestServer(t, up, func(c *Config) { c.MasterAlias = map[string]string{"mod": "base"} })
	newFoo, extra := []byte("foo MODDED and longer"), []byte("brand new table")
	writeFile(t, s.overlayPath("master/mod/Foo.bin"), newFoo)
	writeFile(t, s.overlayPath("master/mod/Extra.bin"), extra)

	code, body, _ := get(t, ts.URL+testBase+"/master/mod/MasterManifest.json")
	if code != 200 {
		t.Fatalf("manifest status %d: %s", code, body)
	}
	var m masterdata.Manifest
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal(err)
	}
	if m.Version != "mod" || len(m.Files) != 3 {
		t.Fatalf("manifest %+v", m)
	}
	byName := map[string]masterdata.ManifestFile{}
	for _, f := range m.Files {
		byName[f.Name] = f
	}
	if f := byName["Foo.bin"]; f.Hash != sha(newFoo) || f.Size != int64(len(newFoo)) {
		t.Fatalf("Foo.bin entry not updated: %+v", f)
	}
	if f := byName["Bar.bin"]; f.Hash != sha(bar) {
		t.Fatalf("Bar.bin entry should be untouched: %+v", f)
	}
	if f := byName["Extra.bin"]; f.Hash != sha(extra) {
		t.Fatalf("Extra.bin entry missing: %+v", f)
	}
	if _, body, _ := get(t, ts.URL+testBase+"/master/mod/Foo.bin"); !bytes.Equal(body, newFoo) {
		t.Fatal("overlay table not served under the alias")
	}
	if _, body, _ := get(t, ts.URL+testBase+"/master/mod/Bar.bin"); !bytes.Equal(body, bar) {
		t.Fatal("untouched table not served from the base version")
	}
	if _, body, _ := get(t, ts.URL+testBase+"/master/base/Foo.bin"); !bytes.Equal(body, foo) {
		t.Fatal("base version changed")
	}
}

func TestConfigValidation(t *testing.T) {
	c := defaultConfig()
	c.Upstream.Root = "https://example.invalid"
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
	bad := []func(*Config){
		func(c *Config) { c.Mode = "mirror" },
		func(c *Config) { c.Upstream.Root = "" },
		func(c *Config) { c.MasterAlias = map[string]string{"a": "a"} },
		func(c *Config) { c.MasterAlias = map[string]string{"a": "b", "b": "c"} },
		func(c *Config) { c.MasterAlias = map[string]string{"a/x": "b"} },
	}
	for i, mut := range bad {
		cc := c
		mut(&cc)
		if err := cc.validate(); err == nil {
			t.Errorf("case %d should be rejected", i)
		}
	}
}

func TestConcurrentRequestsShareOneDownload(t *testing.T) {
	payload := fakeBundle(200000)
	up := newFakeUpstream(t, map[string][]byte{testBase + "/asset/Android/big.bundle": payload})
	_, ts := newTestServer(t, up, nil)
	errs := make(chan string, 24)
	for i := 0; i < 24; i++ {
		go func() {
			code, body, _ := get(t, ts.URL+testBase+"/asset/Android/big.bundle")
			if code != 200 || !bytes.Equal(body, payload) {
				errs <- "bad response"
				return
			}
			errs <- ""
		}()
	}
	for i := 0; i < 24; i++ {
		if e := <-errs; e != "" {
			t.Fatal(e)
		}
	}
	if n := up.hits.Load(); n != 1 {
		t.Fatalf("upstream was hit %d times for 24 concurrent requests, want 1", n)
	}
}
