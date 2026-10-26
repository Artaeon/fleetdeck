package server

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"
)

const sessionLifetime = 8 * time.Hour
const maxSessions = 1024

// Sessions are independent of the API credential, expire on the server, and
// are revoked on logout or process restart. Only token hashes are retained.
type sessionStore struct {
	mu      sync.Mutex
	entries map[[32]byte]sessionEntry
	now     func() time.Time
}

type sessionEntry struct {
	expires time.Time
	device  string
}

func newSessionStore() *sessionStore {
	return &sessionStore{entries: make(map[[32]byte]sessionEntry), now: time.Now}
}

func (s *sessionStore) create() (string, error) {
	return s.createForDevice("")
}

func (s *sessionStore) createForDevice(device string) (string, error) {
	token, err := GenerateAPIToken()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for hash, entry := range s.entries {
		if !now.Before(entry.expires) {
			delete(s.entries, hash)
		}
	}
	if len(s.entries) >= maxSessions {
		return "", fmt.Errorf("session limit reached")
	}
	s.entries[sha256.Sum256([]byte(token))] = sessionEntry{expires: now.Add(sessionLifetime), device: device}
	return token, nil
}

func (s *sessionStore) valid(token string) bool {
	return s.validForDevice(token, "")
}

func (s *sessionStore) validForDevice(token, device string) bool {
	if len(token) != 64 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	hash := sha256.Sum256([]byte(token))
	entry, ok := s.entries[hash]
	if !ok || !s.now().Before(entry.expires) {
		delete(s.entries, hash)
		return false
	}
	return entry.device == device
}

func (s *sessionStore) revoke(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, sha256.Sum256([]byte(token)))
}
