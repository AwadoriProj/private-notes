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
	Session   *sessionEntry
}

type sessionEntry struct {
	UID       int64
	MID       int64
	UName     string
	Email     string
	AccessKey string
	IsNew     bool
}

type memAccount struct {
	UID   int64
	UName string
}

type Store struct {
	mu         sync.Mutex
	otps       map[string]otpEntry
	sessions   map[string]sessionEntry
	accounts   map[string]memAccount
	nextUID    int64
	DB         *db.Store
	sessionTTL time.Duration
}

func NewStore(database *db.Store) *Store {
	return &Store{
		otps:       make(map[string]otpEntry),
		sessions:   make(map[string]sessionEntry),
		accounts:   make(map[string]memAccount),
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
func (s *Store) VerifyOTP(ctx context.Context, ticket, code string) (sessionEntry, bool, error) {
	s.mu.Lock()
	entry, found := s.otps[ticket]
	if !found || time.Now().After(entry.ExpiresAt) {
		s.mu.Unlock()
		return sessionEntry{}, false, nil
	}
	if code != fixedOTP && code != entry.Code {
		s.mu.Unlock()
		return sessionEntry{}, false, nil
	}
	if entry.Session != nil {
		session := *entry.Session
		s.mu.Unlock()
		return session, true, nil
	}
	email := entry.Email
	s.mu.Unlock()

	session, err := s.CreateSession(ctx, email)
	if err != nil {
		return sessionEntry{}, false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	entry, found = s.otps[ticket]
	if !found || time.Now().After(entry.ExpiresAt) {
		return sessionEntry{}, false, nil
	}
	if entry.Session == nil {
		entry.Session = &session
		s.otps[ticket] = entry
	} else {
		session = *entry.Session
	}
	return session, true, nil
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
	acc, exists := s.accounts[email]
	if !exists {
		s.nextUID++
		acc = memAccount{UID: s.nextUID, UName: "user_" + randomHex(6)}
		s.accounts[email] = acc
	}
	entry := sessionEntry{
		UID:       acc.UID,
		MID:       acc.UID,
		UName:     acc.UName,
		Email:     email,
		AccessKey: randomHex(24),
		IsNew:     !exists,
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
		IsNew:     acc.IsNew,
	}, nil
}
func (s *Store) TouchSession(ctx context.Context, accessKey string) error {
	if s.DB == nil {
		return nil
	}
	return s.DB.TouchSession(ctx, accessKey, time.Now().Add(s.sessionTTL))
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
