package config

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Handlers struct {
	Servers []ServerEntry
}

type ServerEntry struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	CDNRoot        string `json:"cdn_root"`
	APIServerRoot  string `json:"api_server_root"`
	ChatServerRoot string `json:"chat_server_root"`
	ATServerRoot   string `json:"at_server_root"`
	LiveServer     string `json:"live_server"`
	AreaID         string `json:"area_id"`
}

func New(servers []ServerEntry) *Handlers {
	return &Handlers{Servers: servers}
}

func requestID() string {
	return strings.ReplaceAll(uuid.NewString(), "-", "")
}

func originalHost(r *http.Request) string {
	if h := r.Header.Get("X-Private-Notes-Original-Host"); h != "" {
		return strings.ToLower(h)
	}
	return strings.ToLower(r.Host)
}

func writeJSON(w http.ResponseWriter, ticket bool, body map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if ticket {
		w.Header().Set("X-Ticket-Status", "1")
	}
	_ = json.NewEncoder(w).Encode(body)
}

func loginStyle(w http.ResponseWriter, data interface{}) {
	body := map[string]interface{}{
		"code":       0,
		"request_id": requestID(),
		"timestamp":  time.Now().UnixMilli(),
		"message":    "Permintaan terkirim",
	}
	if data != nil {
		body["data"] = data
	}
	writeJSON(w, true, body)
}

func sdkStyle(w http.ResponseWriter, data interface{}) {
	body := map[string]interface{}{
		"request_id": requestID(),
		"code":       0,
		"message":    "Permintaan Berhasil",
	}
	if data != nil {
		body["data"] = data
	}
	writeJSON(w, true, body)
}

func supportStyle(w http.ResponseWriter, data interface{}, extra map[string]interface{}) {
	body := map[string]interface{}{
		"code":       0,
		"message":    "success",
		"request_id": requestID(),
		"ts":         time.Now().UnixMilli(),
		"success":    true,
	}
	if data != nil {
		body["data"] = data
	}
	for k, v := range extra {
		body[k] = v
	}
	writeJSON(w, false, body)
}

func legacyStyle(w http.ResponseWriter, data interface{}, withSnake bool) {
	id := uuid.NewString()
	body := map[string]interface{}{
		"code":      "0000",
		"data":      data,
		"msg":       "success",
		"requestId": id,
		"timestamp": time.Now().UnixMilli(),
	}
	if withSnake {
		body["request_id"] = id
	}
	writeJSON(w, false, body)
}

func (h *Handlers) Activate(w http.ResponseWriter, r *http.Request) {
	loginStyle(w, nil)
}

func (h *Handlers) Config(w http.ResponseWriter, r *http.Request) {
	if strings.Contains(originalHost(r), "game-support") {
		supportStyle(w, agreementConfigJSON, map[string]interface{}{"success": true})
		return
	}
	loginStyle(w, loginConfigJSON)
}

func (h *Handlers) CountryList(w http.ResponseWriter, r *http.Request) {
	loginStyle(w, countryListJSON)
}

func (h *Handlers) ServerList(w http.ResponseWriter, r *http.Request) {
	loginStyle(w, map[string]interface{}{"server_list": h.Servers})
}

func (h *Handlers) ABTest(w http.ResponseWriter, r *http.Request) {
	sdkStyle(w, map[string]interface{}{"is_new_ui": false, "ab_test_info": nil})
}

func (h *Handlers) OverseasConfig(w http.ResponseWriter, r *http.Request) {
	sdkStyle(w, overseasConfigJSON)
}

func (h *Handlers) NoticeList(w http.ResponseWriter, r *http.Request) {
	sdkStyle(w, map[string]interface{}{"show_limit_num": 5, "notices": []interface{}{}})
}

func (h *Handlers) GameSupportConfig(w http.ResponseWriter, r *http.Request) {
	supportStyle(w, gameSupportConfigJSON, nil)
}

func (h *Handlers) SyncAgreementStatus(w http.ResponseWriter, r *http.Request) {
	supportStyle(w, nil, map[string]interface{}{"success": true})
}

func (h *Handlers) NetcheckSafe(w http.ResponseWriter, r *http.Request) {
	supportStyle(w, netcheckSafeData, nil)
}

func (h *Handlers) RealtimeConf(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	var conf map[string]interface{}
	_ = json.Unmarshal(realtimeConfJSON, &conf)
	conf["serverRequestId"] = requestID()
	conf["timestamp"] = strconv.FormatInt(time.Now().UnixMilli(), 10)
	conf["clientRequestId"] = r.Form.Get("client_request_uuid")
	writeJSON(w, false, conf)
}

func (h *Handlers) RealtimeHeartbeat(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"code": "0"})
}

func (h *Handlers) FeatureFlag(w http.ResponseWriter, r *http.Request) {
	legacyStyle(w, featureFlagJSON, false)
}

func (h *Handlers) CloudStorageConfig(w http.ResponseWriter, r *http.Request) {
	legacyStyle(w, cloudStorageJSON, true)
}
