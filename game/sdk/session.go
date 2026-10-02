package sdk

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"private-notes/game/db"
)

type otpEntry struct {
	Email     string
	Code      string
	ExpiresAt time.Time
}

type sessionEntry struct {
	UID       int64
	MID       int64
	UName     string
	Email     string
	AccessKey string
}

type Store struct {
	mu         sync.Mutex
	otps       map[string]otpEntry
	sessions   map[string]sessionEntry
	nextUID    int64
	DB         *db.Store
	sessionTTL time.Duration
}

func NewStore(database *db.Store) *Store {
	return &Store{
		otps:       make(map[string]otpEntry),
		sessions:   make(map[string]sessionEntry),
		nextUID:    100000,
		DB:         database,
		sessionTTL: 30 * 24 * time.Hour,
	}
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

const fixedOTP = "000000"

func (s *Store) CreateOTP(email string) (ticket string, ttl int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ticket = randomHex(16)
	s.otps[ticket] = otpEntry{Email: email, Code: fixedOTP, ExpiresAt: time.Now().Add(5 * time.Minute)}
	return ticket, 270
}

func (s *Store) VerifyOTP(ticket, code string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.otps[ticket]
	if !ok || time.Now().After(entry.ExpiresAt) {
		return false
	}
	if code != fixedOTP && code != entry.Code {
		return false
	}
	delete(s.otps, ticket)
	return true
}

func (s *Store) CreateSession(ctx context.Context, email string) (sessionEntry, error) {
	if s.DB != nil {
		return s.createSessionDB(ctx, email)
	}
	return s.createSessionMemory(email), nil
}

func (s *Store) createSessionMemory(email string) sessionEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextUID++
	entry := sessionEntry{
		UID:       s.nextUID,
		MID:       s.nextUID,
		UName:     "user_" + randomHex(6),
		Email:     email,
		AccessKey: randomHex(24),
	}
	s.sessions[entry.AccessKey] = entry
	return entry
}

func (s *Store) createSessionDB(ctx context.Context, email string) (sessionEntry, error) {
	acc, err := s.DB.UpsertAccount(ctx, email, "user_"+randomHex(6))
	if err != nil {
		return sessionEntry{}, err
	}
	accessKey := randomHex(24)
	if err := s.DB.SaveSession(ctx, accessKey, acc.UID, time.Now().Add(s.sessionTTL)); err != nil {
		return sessionEntry{}, err
	}
	return sessionEntry{
		UID:       acc.UID,
		MID:       acc.UID,
		UName:     acc.UName,
		Email:     acc.Email,
		AccessKey: accessKey,
	}, nil
}

func (s *Store) LookupByAccessKey(ctx context.Context, key string) (sessionEntry, bool, error) {
	if s.DB != nil {
		found, ok, err := s.DB.LookupSession(ctx, key)
		if err != nil || !ok {
			return sessionEntry{}, ok, err
		}
		return sessionEntry{UID: found.UID, MID: found.UID, UName: found.UName, Email: found.Email, AccessKey: key}, true, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.sessions[key]
	return e, ok, nil
}