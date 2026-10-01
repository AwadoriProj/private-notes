package sdk

import (
	"net/http"
	"time"

	"private-notes/game/db"
)

type Server struct {
	Keys  *RSAKeys
	Store *Store
}

func NewServer(keys *RSAKeys, database *db.Store) *Server {
	return &Server{Keys: keys, Store: NewStore(database)}
}

func (s *Server) RSAPublic(w http.ResponseWriter, r *http.Request) {
	writeOK(w, map[string]string{
		"rsa_key": s.Keys.PEM,
		"hash":    s.Keys.Hash,
	})
}

func (s *Server) OTPSend(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	encrypted := r.Form.Get("email")

	email, err := s.Keys.DecryptEmail(encrypted)
	if err != nil {
		writeErr(w, -1, "failed to decrypt email")
		return
	}

	ticket, ttl := s.Store.CreateOTP(email)
	writeOK(w, map[string]interface{}{
		"email_ticket": ticket,
		"ttl":          ttl,
	})
}

func (s *Server) OTPVerifyLogin(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	ticket := r.Form.Get("email_ticket")
	code := r.Form.Get("email_code")
	email := r.Form.Get("email")

	if !s.Store.VerifyOTP(ticket, code, email) {
		writeErr(w, -2, "invalid otp code")
		return
	}

	session, err := s.Store.CreateSession(r.Context(), email)
	if err != nil {
		writeErr(w, -5, "failed to create session")
		return
	}

	now := time.Now()
	idToken, err := s.Keys.SignJWT(map[string]interface{}{
		"sub":          session.UName,
		"iss":          "private-notes",
		"aud":          "private-notes-client",
		"iat":          now.Unix(),
		"exp":          now.Add(72 * time.Hour).Unix(),
		"access_token": session.AccessKey,
	})
	if err != nil {
		writeErr(w, -3, "failed to create token")
		return
	}

	writeOK(w, map[string]interface{}{
		"u_name":        session.UName,
		"expires":       now.Add(72 * time.Hour).UnixMilli(),
		"id_token":      idToken,
		"hashed_email":  sha256Hex(email),
		"mid":           session.MID,
		"need_realname": false,
		"s_face":        "",
		"uid":           session.UID,
		"face":          "",
		"is_tourist":    false,
		"hashed_tel":    "",
		"is_new_user":   1,
		"access_key":    session.AccessKey,
	})
}

func (s *Server) CreateRole(w http.ResponseWriter, r *http.Request) {
	writeOK(w, nil)
}

func (s *Server) NotifyZone(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	accessKey := r.Form.Get("access_key")
	_, ok, err := s.Store.LookupByAccessKey(r.Context(), accessKey)
	if err != nil {
		writeErr(w, -6, "session lookup failed")
		return
	}
	if !ok {
		writeErr(w, -4, "unknown access_key")
		return
	}
	writeOK(w, nil)
}

func (s *Server) SyncAgreementStatus(w http.ResponseWriter, r *http.Request) {
	writeEnvelope(w, 0, "success", nil)
}