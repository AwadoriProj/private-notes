package logging

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const maxBodyLog = 4096

func redactedHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, v := range h {
		lk := strings.ToLower(k)
		if strings.Contains(lk, "authorization") || strings.Contains(lk, "cookie") {
			out[k] = "<redacted>"
			continue
		}
		out[k] = strings.Join(v, ", ")
	}
	return out
}

func bodyPreview(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	n := len(body)
	truncated := n > maxBodyLog
	preview := body
	if truncated {
		preview = body[:maxBodyLog]
	}
	s := string(preview)
	if truncated {
		s += "...(truncated)"
	}
	return s
}

func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		originalHost := r.Header.Get("X-Private-Notes-Original-Host")
		if originalHost == "" {
			originalHost = r.Host
		}

		var bodyBytes []byte
		if r.Body != nil {
			bodyBytes, _ = io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}

		rec := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)

		log.Printf(
			"%s %s%s host=%s status=%d dur=%s body=%q",
			r.Method, originalHost, r.URL.Path, r.Host, rec.status, time.Since(start), bodyPreview(bodyBytes),
		)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func Unmapped(w http.ResponseWriter, r *http.Request) {
	originalHost := r.Header.Get("X-Private-Notes-Original-Host")
	if originalHost == "" {
		originalHost = r.Host
	}

	var bodyBytes []byte
	if r.Body != nil {
		bodyBytes, _ = io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}
	_ = r.ParseForm()

	log.Printf(
		"UNMAPPED %s %s%s\n  query=%v\n  form=%v\n  headers=%v\n  body=%q",
		r.Method, originalHost, r.URL.Path, r.URL.Query(), r.Form, redactedHeaders(r.Header), bodyPreview(bodyBytes),
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"code":    0,
		"message": "ok",
		"data":    map[string]interface{}{},
	})
}