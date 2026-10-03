package sdk

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Envelope struct {
	Code      int         `json:"code"`
	RequestID string      `json:"request_id"`
	Timestamp int64       `json:"timestamp"`
	Message   string      `json:"message"`
	Data      interface{} `json:"data,omitempty"`
}

func writeOK(w http.ResponseWriter, data interface{}) {
	writeEnvelope(w, 0, "Permintaan terkirim", data)
}

func writeErr(w http.ResponseWriter, code int, message string) {
	writeEnvelope(w, code, message, nil)
}

func writeEnvelope(w http.ResponseWriter, code int, message string, data interface{}) {
	env := Envelope{
		Code:      code,
		RequestID: strings.ReplaceAll(uuid.NewString(), "-", ""),
		Timestamp: time.Now().UnixMilli(),
		Message:   message,
		Data:      data,
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Ticket-Status", "1")
	_ = json.NewEncoder(w).Encode(env)
}
