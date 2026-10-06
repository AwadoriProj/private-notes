package grpcapi

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

const defaultPlayerName = "Player"

const credentialAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

type player struct {
	ID         string `json:"id"`
	Credential string `json:"credential"`
	SdkUID     string `json:"sdk_uid"`
	ProfileID  int64  `json:"profile_id"`
	DeviceID   string `json:"device_id"`
	Name       string `json:"name"`
	CreatedAt  int64  `json:"created_at"`
	UpdatedAt  int64  `json:"updated_at"`
}

type playerStore struct {
	mu    sync.Mutex
	path  string
	byID  map[string]*player
	bySDK map[string]*player
	used  map[int64]bool
	now   func() time.Time
}

func newPlayerStore(path string, now func() time.Time) (*playerStore, error) {
	s := &playerStore{
		path:  path,
		byID:  make(map[string]*player),
		bySDK: make(map[string]*player),
		used:  make(map[int64]bool),
		now:   now,
	}
	if path == "" {
		return s, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	var list []*player
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	for _, p := range list {
		s.byID[p.ID] = p
		s.bySDK[p.SdkUID] = p
		s.used[p.ProfileID] = true
	}
	return s, nil
}

func randomCredential() (string, error) {
	out := make([]byte, 32)
	max := big.NewInt(int64(len(credentialAlphabet)))
	for i := range out {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = credentialAlphabet[n.Int64()]
	}
	return string(out), nil
}

func (s *playerStore) newProfileID() (int64, error) {
	span := big.NewInt(90_000_000_000)
	for {
		n, err := rand.Int(rand.Reader, span)
		if err != nil {
			return 0, err
		}
		id := n.Int64() + 10_000_000_000
		if !s.used[id] {
			return id, nil
		}
	}
}

func (s *playerStore) persistLocked() error {
	if s.path == "" {
		return nil
	}
	list := make([]*player, 0, len(s.byID))
	for _, p := range s.byID {
		list = append(list, p)
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(s.path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *playerStore) exists(sdkUID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.bySDK[sdkUID]
	return ok
}

func (s *playerStore) login(sdkUID, deviceID string) (player, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.bySDK[sdkUID]; ok {
		if deviceID != "" && p.DeviceID != deviceID {
			p.DeviceID = deviceID
			if err := s.persistLocked(); err != nil {
				return player{}, err
			}
		}
		return *p, nil
	}
	id, err := uuid.NewV7()
	if err != nil {
		return player{}, err
	}
	cred, err := randomCredential()
	if err != nil {
		return player{}, err
	}
	profileID, err := s.newProfileID()
	if err != nil {
		return player{}, err
	}
	now := s.now().Unix()
	p := &player{
		ID:         id.String(),
		Credential: cred,
		SdkUID:     sdkUID,
		ProfileID:  profileID,
		DeviceID:   deviceID,
		Name:       defaultPlayerName,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	s.byID[p.ID] = p
	s.bySDK[sdkUID] = p
	s.used[profileID] = true
	if err := s.persistLocked(); err != nil {
		delete(s.byID, p.ID)
		delete(s.bySDK, sdkUID)
		delete(s.used, profileID)
		return player{}, err
	}
	return *p, nil
}

func (s *playerStore) authenticate(id, credential string) (player, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.byID[id]
	if !ok || credential == "" || p.Credential != credential {
		return player{}, false
	}
	return *p, true
}

func (s *playerStore) rename(id, name string) (player, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.byID[id]
	if !ok {
		return player{}, errors.New("unknown player")
	}
	p.Name = name
	p.UpdatedAt = s.now().Unix()
	if err := s.persistLocked(); err != nil {
		return player{}, err
	}
	return *p, nil
}
