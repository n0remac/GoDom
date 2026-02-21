package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/n0remac/GoDom/database"
	"golang.org/x/crypto/bcrypt"
)

type SQLiteStore struct {
	ds *database.DocumentStore
	mu sync.Mutex
}

func NewSQLiteStore(ds *database.DocumentStore) *SQLiteStore {
	return &SQLiteStore{ds: ds}
}

const (
	userPrefix             = "user:"
	emailPrefix            = "email:"
	sessionPrefix          = "session:"
	invitePrefix           = "invite:"
	registrationModeConfig = "config:registration_mode"
)

func genRandID(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// CreateUser creates a new user and stores an email->id mapping to allow lookup by email.
func (s *SQLiteStore) CreateUser(email, password string) (*User, error) {
	return s.createUserWithRole(email, password, RoleMember)
}

func (s *SQLiteStore) createUserWithRole(email, password, role string) (*User, error) {
	email = normalizeEmail(email)
	role, err := normalizeRole(role)
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	s.mu.Lock()
	defer s.mu.Unlock()

	if b, _ := s.ds.Get(ctx, emailKey(email)); b != nil {
		return nil, ErrUserExists
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
		Role:         role,
		CreatedAt:    time.Now(),
	}
	ub, err := json.Marshal(u)
	if err != nil {
		return nil, err
	}
	if err := s.ds.Put(ctx, userKey(id), ub); err != nil {
		return nil, err
	}
	// store email -> id mapping
	idx, _ := json.Marshal(map[string]string{"id": id})
	if err := s.ds.Put(ctx, emailKey(email), idx); err != nil {
		_ = s.ds.Delete(ctx, userKey(id))
		return nil, err
	}
	return u, nil
}

func (s *SQLiteStore) EnsureAdmin(email, password string) (bool, error) {
	user, err := s.GetByEmail(email)
	if err == nil {
		if user.Role == RoleAdmin {
			return false, nil
		}
		return false, s.UpdateRole(user.ID, RoleAdmin)
	}
	if !errors.Is(err, ErrUserNotFound) {
		return false, err
	}
	_, err = s.createUserWithRole(email, password, RoleAdmin)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *SQLiteStore) GetByEmail(email string) (*User, error) {
	email = normalizeEmail(email)
	ctx := context.Background()
	b, err := s.ds.Get(ctx, emailKey(email))
	if err != nil {
		return nil, err
	}
	if b != nil {
		return s.userFromEmailIndex(email, b)
	}

	// Backfill support for legacy mixed-case records created before email normalization.
	ids, err := s.listIDsWithPrefix(userPrefix)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		userID := strings.TrimPrefix(id, userPrefix)
		user, err := s.GetByID(userID)
		if err != nil {
			continue
		}
		if normalizeEmail(user.Email) != email {
			continue
		}
		_ = s.reindexUserEmail(user, email)
		user.Email = email
		return user, nil
	}

	return nil, ErrUserNotFound
}

func (s *SQLiteStore) GetByID(id string) (*User, error) {
	ctx := context.Background()
	b, err := s.ds.Get(ctx, userKey(id))
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, ErrUserNotFound
	}
	var u User
	if err := json.Unmarshal(b, &u); err != nil {
		return nil, err
	}
	if u.Role == "" {
		u.Role = RoleMember
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

func (s *SQLiteStore) ListUsers() ([]*User, error) {
	ids, err := s.listIDsWithPrefix(userPrefix)
	if err != nil {
		return nil, err
	}
	users := make([]*User, 0, len(ids))
	for _, id := range ids {
		userID := strings.TrimPrefix(id, userPrefix)
		user, err := s.GetByID(userID)
		if err != nil {
			continue
		}
		users = append(users, user)
	}
	sort.Slice(users, func(i, j int) bool {
		return users[i].Email < users[j].Email
	})
	return users, nil
}

func (s *SQLiteStore) UpdateRole(id, role string) error {
	role, err := normalizeRole(role)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	user, err := s.GetByID(id)
	if err != nil {
		return err
	}
	user.Role = role
	b, err := json.Marshal(user)
	if err != nil {
		return err
	}
	return s.ds.Put(context.Background(), userKey(id), b)
}

func (s *SQLiteStore) DeleteUser(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	user, err := s.GetByID(id)
	if err != nil {
		return err
	}
	ctx := context.Background()
	if err := s.ds.Delete(ctx, userKey(id)); err != nil {
		return err
	}
	return s.ds.Delete(ctx, emailKey(user.Email))
}

func (s *SQLiteStore) CountByRole(role string) (int, error) {
	role, err := normalizeRole(role)
	if err != nil {
		return 0, err
	}
	users, err := s.ListUsers()
	if err != nil {
		return 0, err
	}
	count := 0
	for _, user := range users {
		if user.Role == role {
			count++
		}
	}
	return count, nil
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
	if err := s.ds.Put(ctx, sessionKey(id), b); err != nil {
		return nil, err
	}
	return sess, nil
}

func (s *SQLiteStore) Get(id string) (*Session, bool) {
	ctx := context.Background()
	b, err := s.ds.Get(ctx, sessionKey(id))
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
	_ = s.ds.Delete(ctx, sessionKey(id))
}

func (s *SQLiteStore) GetRegistrationMode() (RegistrationMode, error) {
	ctx := context.Background()
	b, err := s.ds.Get(ctx, registrationModeConfig)
	if err != nil {
		return RegistrationOpen, err
	}
	if b == nil {
		return RegistrationOpen, nil
	}
	var m struct {
		Mode RegistrationMode `json:"mode"`
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return RegistrationOpen, err
	}
	if !isValidRegistrationMode(m.Mode) {
		return RegistrationOpen, nil
	}
	return m.Mode, nil
}

func (s *SQLiteStore) SetRegistrationMode(mode RegistrationMode) error {
	if !isValidRegistrationMode(mode) {
		return ErrInvalidRegistrationMode
	}
	b, err := json.Marshal(map[string]string{"mode": string(mode)})
	if err != nil {
		return err
	}
	return s.ds.Put(context.Background(), registrationModeConfig, b)
}

func (s *SQLiteStore) CreateInvite(createdBy string, ttl time.Duration) (*Invite, error) {
	token, err := genRandID(24)
	if err != nil {
		return nil, err
	}
	invite := &Invite{
		Token:     token,
		CreatedBy: createdBy,
		CreatedAt: time.Now(),
	}
	if ttl > 0 {
		exp := time.Now().Add(ttl)
		invite.ExpiresAt = &exp
	}
	if err := s.putInvite(invite); err != nil {
		return nil, err
	}
	return invite, nil
}

func (s *SQLiteStore) GetInvite(token string) (*Invite, error) {
	ctx := context.Background()
	b, err := s.ds.Get(ctx, inviteKey(token))
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, ErrInviteNotFound
	}
	var invite Invite
	if err := json.Unmarshal(b, &invite); err != nil {
		return nil, err
	}
	return &invite, nil
}

func (s *SQLiteStore) ConsumeInvite(token, usedBy string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	invite, err := s.GetInvite(token)
	if err != nil {
		return err
	}
	now := time.Now()
	if invite.IsUsed() {
		return ErrInviteUsed
	}
	if invite.IsExpired(now) {
		return ErrInviteExpired
	}
	invite.UsedAt = &now
	invite.UsedBy = usedBy
	return s.putInvite(invite)
}

func (s *SQLiteStore) ListInvites() ([]*Invite, error) {
	ids, err := s.listIDsWithPrefix(invitePrefix)
	if err != nil {
		return nil, err
	}
	invites := make([]*Invite, 0, len(ids))
	for _, id := range ids {
		token := strings.TrimPrefix(id, invitePrefix)
		invite, err := s.GetInvite(token)
		if err != nil {
			continue
		}
		invites = append(invites, invite)
	}
	sort.Slice(invites, func(i, j int) bool {
		return invites[i].CreatedAt.After(invites[j].CreatedAt)
	})
	return invites, nil
}

func (s *SQLiteStore) putInvite(invite *Invite) error {
	b, err := json.Marshal(invite)
	if err != nil {
		return err
	}
	return s.ds.Put(context.Background(), inviteKey(invite.Token), b)
}

func (s *SQLiteStore) listIDsWithPrefix(prefix string) ([]string, error) {
	ids, err := s.ds.List(context.Background())
	if err != nil {
		return nil, err
	}
	filtered := make([]string, 0, len(ids))
	for _, id := range ids {
		if strings.HasPrefix(id, prefix) {
			filtered = append(filtered, id)
		}
	}
	return filtered, nil
}

func normalizeRole(role string) (string, error) {
	role = strings.ToLower(strings.TrimSpace(role))
	switch role {
	case RoleMember, RoleAdmin:
		return role, nil
	default:
		return "", ErrInvalidRole
	}
}

func (s *SQLiteStore) userFromEmailIndex(normalizedEmail string, index []byte) (*User, error) {
	var m map[string]string
	if err := json.Unmarshal(index, &m); err != nil {
		return nil, err
	}
	id := m["id"]
	if id == "" {
		return nil, ErrUserNotFound
	}
	user, err := s.GetByID(id)
	if err != nil {
		return nil, err
	}
	if user.Email != normalizedEmail {
		_ = s.reindexUserEmail(user, normalizedEmail)
		user.Email = normalizedEmail
	}
	return user, nil
}

func (s *SQLiteStore) reindexUserEmail(user *User, normalizedEmail string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	fresh, err := s.GetByID(user.ID)
	if err != nil {
		return err
	}
	oldEmail := fresh.Email
	if oldEmail != normalizedEmail {
		fresh.Email = normalizedEmail
		b, err := json.Marshal(fresh)
		if err != nil {
			return err
		}
		if err := s.ds.Put(context.Background(), userKey(fresh.ID), b); err != nil {
			return err
		}
	}
	idx, err := json.Marshal(map[string]string{"id": fresh.ID})
	if err != nil {
		return err
	}
	if err := s.ds.Put(context.Background(), emailKey(normalizedEmail), idx); err != nil {
		return err
	}
	if oldEmail != "" && oldEmail != normalizedEmail {
		_ = s.ds.Delete(context.Background(), emailKey(oldEmail))
	}
	return nil
}

func isValidRegistrationMode(mode RegistrationMode) bool {
	switch mode {
	case RegistrationOpen, RegistrationInviteOnly, RegistrationClosed:
		return true
	default:
		return false
	}
}

func userKey(id string) string {
	return userPrefix + id
}

func emailKey(email string) string {
	return emailPrefix + email
}

func sessionKey(id string) string {
	return sessionPrefix + id
}

func inviteKey(token string) string {
	return invitePrefix + token
}
