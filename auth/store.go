package auth

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID            string
	Email         string
	PasswordHash  []byte
	Role          string
	CreatedAt     time.Time
	FailedAttemps int
	LockedUntil   time.Time
}

const (
	RoleMember = "member"
	RoleAdmin  = "admin"
)

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
var ErrInvalidRole = errors.New("invalid role")
var ErrInviteNotFound = errors.New("invite not found")
var ErrInviteUsed = errors.New("invite already used")
var ErrInviteExpired = errors.New("invite expired")
var ErrInvalidRegistrationMode = errors.New("invalid registration mode")

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
		Role:         RoleMember,
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

func (s *InMemoryUserStore) ListUsers() ([]*User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	users := make([]*User, 0, len(s.byID))
	for _, user := range s.byID {
		u := *user
		if u.Role == "" {
			u.Role = RoleMember
		}
		users = append(users, &u)
	}
	sort.Slice(users, func(i, j int) bool {
		return users[i].Email < users[j].Email
	})
	return users, nil
}

func (s *InMemoryUserStore) UpdateRole(id, role string) error {
	if role != RoleMember && role != RoleAdmin {
		return ErrInvalidRole
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.byID[id]
	if !ok {
		return ErrUserNotFound
	}
	user.Role = role
	return nil
}

func (s *InMemoryUserStore) DeleteUser(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	user, ok := s.byID[id]
	if !ok {
		return ErrUserNotFound
	}
	delete(s.byEmail, user.Email)
	delete(s.byID, id)
	return nil
}

func (s *InMemoryUserStore) CountByRole(role string) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, user := range s.byID {
		userRole := user.Role
		if userRole == "" {
			userRole = RoleMember
		}
		if userRole == role {
			count++
		}
	}
	return count, nil
}

// VerifyPassword returns nil on success
func (s *InMemoryUserStore) VerifyPassword(email, password string) error {
	u, err := s.GetByEmail(email)
	if err != nil {
		return err
	}
	return bcrypt.CompareHashAndPassword(u.PasswordHash, []byte(password))
}
