package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/n0remac/GoDom/database"
	"golang.org/x/crypto/bcrypt"
)

type SQLiteStore struct {
	ds *database.DocumentStore
}

func NewSQLiteStore(ds *database.DocumentStore) *SQLiteStore {
	return &SQLiteStore{ds: ds}
}

func genRandID(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// CreateUser creates a new user and stores an email->id mapping to allow lookup by email.
func (s *SQLiteStore) CreateUser(email, password string) (*User, error) {
	ctx := context.Background()
	// check email exists
	if b, _ := s.ds.Get(ctx, "email:"+email); b != nil {
		return nil, errors.New("user already exists")
	}
	id, err := genRandID(16)
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
	ub, err := json.Marshal(u)
	if err != nil {
		return nil, err
	}
	if err := s.ds.Put(ctx, "user:"+id, ub); err != nil {
		return nil, err
	}
	// store email -> id mapping
	idx, _ := json.Marshal(map[string]string{"id": id})
	if err := s.ds.Put(ctx, "email:"+email, idx); err != nil {
		return nil, err
	}
	return u, nil
}

func (s *SQLiteStore) GetByEmail(email string) (*User, error) {
	ctx := context.Background()
	b, err := s.ds.Get(ctx, "email:"+email)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, errors.New("user not found")
	}
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	id := m["id"]
	return s.GetByID(id)
}

func (s *SQLiteStore) GetByID(id string) (*User, error) {
	ctx := context.Background()
	b, err := s.ds.Get(ctx, "user:"+id)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, errors.New("user not found")
	}
	var u User
	if err := json.Unmarshal(b, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *SQLiteStore) VerifyPassword(email, password string) error {
	u, err := s.GetByEmail(email)
	if err != nil {
		return err
	}
	return bcrypt.CompareHashAndPassword(u.PasswordHash, []byte(password))
}

// Sessions
func (s *SQLiteStore) Create(userID string, ttl time.Duration) (*Session, error) {
	ctx := context.Background()
	id, err := genRandID(24)
	if err != nil {
		return nil, err
	}
	sess := &Session{
		ID:        id,
		UserID:    userID,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(ttl),
	}
	b, err := json.Marshal(sess)
	if err != nil {
		return nil, err
	}
	if err := s.ds.Put(ctx, "session:"+id, b); err != nil {
		return nil, err
	}
	return sess, nil
}

func (s *SQLiteStore) Get(id string) (*Session, bool) {
	ctx := context.Background()
	b, err := s.ds.Get(ctx, "session:"+id)
	if err != nil || b == nil {
		return nil, false
	}
	var sess Session
	if err := json.Unmarshal(b, &sess); err != nil {
		return nil, false
	}
	if time.Now().After(sess.ExpiresAt) {
		s.Delete(id)
		return nil, false
	}
	return &sess, true
}

func (s *SQLiteStore) Delete(id string) {
	ctx := context.Background()
	_ = s.ds.Delete(ctx, "session:"+id)
}
