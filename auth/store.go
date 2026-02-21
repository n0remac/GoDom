package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID            string
	Email         string
	PasswordHash  []byte
	CreatedAt     time.Time
	FailedAttemps int
	LockedUntil   time.Time
}

type UserStore interface {
	CreateUser(email, password string) (*User, error)
	GetByEmail(email string) (*User, error)
}

type InMemoryUserStore struct {
	mu      sync.RWMutex
	byID    map[string]*User
	byEmail map[string]*User
}

func NewInMemoryUserStore() *InMemoryUserStore {
	return &InMemoryUserStore{
		byID:    make(map[string]*User),
		byEmail: make(map[string]*User),
	}
}

var ErrUserExists = errors.New("user already exists")
var ErrUserNotFound = errors.New("user not found")

func genID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (s *InMemoryUserStore) CreateUser(email, password string) (*User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byEmail[email]; ok {
		return nil, ErrUserExists
	}
	id, err := genID()
	if err != nil {
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	u := &User{
		ID:           id,
		Email:        email,
		PasswordHash: hash,
		CreatedAt:    time.Now(),
	}
	s.byID[id] = u
	s.byEmail[email] = u
	return u, nil
}

func (s *InMemoryUserStore) GetByEmail(email string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.byEmail[email]
	if !ok {
		return nil, ErrUserNotFound
	}
	return u, nil
}

func (s *InMemoryUserStore) GetByID(id string) (*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.byID[id]
	if !ok {
		return nil, ErrUserNotFound
	}
	return u, nil
}

// VerifyPassword returns nil on success
func (s *InMemoryUserStore) VerifyPassword(email, password string) error {
	u, err := s.GetByEmail(email)
	if err != nil {
		return err
	}
	return bcrypt.CompareHashAndPassword(u.PasswordHash, []byte(password))
}
