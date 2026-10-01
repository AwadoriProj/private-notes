package config

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type Handlers struct {
	Servers []ServerEntry
}

type ServerEntry struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func New(servers []ServerEntry) *Handlers {
	return &Handlers{Servers: servers}
}

func (h *Handlers) write(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"code":       0,
		"request_id": uuid.NewString(),
		"timestamp":  time.Now().UnixMilli(),
		"message":    "Permintaan terkirim",
		"data":       data,
	})
}

// ServerList is a guess, not a captured schema: the real "** Region" picker
// endpoint has not been found in traffic yet. Shape and field names here are
// placeholders until the real response is captured. See PORTING.md.
func (h *Handlers) ServerList(w http.ResponseWriter, r *http.Request) {
	h.write(w, map[string]interface{}{"server_list": h.Servers})
}

func (h *Handlers) Config(w http.ResponseWriter, r *http.Request) {
	h.write(w, map[string]interface{}{})
}

func (h *Handlers) NetcheckSafe(w http.ResponseWriter, r *http.Request) {
	h.write(w, map[string]interface{}{"safe": true})
}

func (h *Handlers) NoticeList(w http.ResponseWriter, r *http.Request) {
	h.write(w, map[string]interface{}{"notices": []interface{}{}})
}

func (h *Handlers) ABTest(w http.ResponseWriter, r *http.Request) {
	h.write(w, map[string]interface{}{})
}

func (h *Handlers) CloudStorageConfig(w http.ResponseWriter, r *http.Request) {
	h.write(w, map[string]interface{}{})
}

func (h *Handlers) FeatureFlag(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"code": 0, "data": map[string]interface{}{}})
}