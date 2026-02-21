package auth

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"os"
	"sync"
	"time"
)

type Session struct {
	ID        string
	UserID    string
	CreatedAt time.Time
	ExpiresAt time.Time
}

type SessionStore struct {
	mu   sync.RWMutex
	data map[string]*Session
}

func NewSessionStore() *SessionStore {
	return &SessionStore{
		data: make(map[string]*Session),
	}
}

func genSessionID() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *SessionStore) Create(userID string, ttl time.Duration) (*Session, error) {
	id, err := genSessionID()
	if err != nil {
		return nil, err
	}
	sess := &Session{
		ID:        id,
		UserID:    userID,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(ttl),
	}
	s.mu.Lock()
	s.data[id] = sess
	s.mu.Unlock()
	return sess, nil
}

func (s *SessionStore) Get(id string) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sess, ok := s.data[id]
	if !ok {
		return nil, false
	}
	if time.Now().After(sess.ExpiresAt) {
		return nil, false
	}
	return sess, true
}

func (s *SessionStore) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, id)
}

const sessionCookieName = "gdsess"

func setSessionCookie(w http.ResponseWriter, id string, ttl time.Duration) {
	cookie := &http.Cookie{
		Name:     sessionCookieName,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		Expires:  time.Now().Add(ttl),
		SameSite: http.SameSiteStrictMode,
	}
	if os.Getenv("ENVIRONMENT") == "production" {
		cookie.Secure = true
	}
	http.SetCookie(w, cookie)
}

func clearSessionCookie(w http.ResponseWriter) {
	cookie := &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		SameSite: http.SameSiteStrictMode,
	}
	if os.Getenv("ENVIRONMENT") == "production" {
		cookie.Secure = true
	}
	http.SetCookie(w, cookie)
}

func getSessionIDFromRequest(r *http.Request) (string, bool) {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return "", false
	}
	return c.Value, true
}
