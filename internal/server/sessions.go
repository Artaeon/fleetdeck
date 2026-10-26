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
	entries map[[32]byte]time.Time
	now     func() time.Time
}

func newSessionStore() *sessionStore {
	return &sessionStore{entries: make(map[[32]byte]time.Time), now: time.Now}
}

func (s *sessionStore) create() (string, error) {
	token, err := GenerateAPIToken()
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for hash, expires := range s.entries {
		if !now.Before(expires) {
			delete(s.entries, hash)
		}
	}
	if len(s.entries) >= maxSessions {
		return "", fmt.Errorf("session limit reached")
	}
	s.entries[sha256.Sum256([]byte(token))] = now.Add(sessionLifetime)
	return token, nil
}

func (s *sessionStore) valid(token string) bool {
	if len(token) != 64 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	hash := sha256.Sum256([]byte(token))
	expires, ok := s.entries[hash]
	if !ok || !s.now().Before(expires) {
		delete(s.entries, hash)
		return false
	}
	return true
}

func (s *sessionStore) revoke(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, sha256.Sum256([]byte(token)))
}
