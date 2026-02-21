package auth

import (
	"sort"
	"sync"
	"time"
)

type InMemoryRegistrationStore struct {
	mu   sync.RWMutex
	mode RegistrationMode
}

func NewInMemoryRegistrationStore() *InMemoryRegistrationStore {
	return &InMemoryRegistrationStore{
		mode: RegistrationOpen,
	}
}

func (s *InMemoryRegistrationStore) GetRegistrationMode() (RegistrationMode, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.mode == "" {
		return RegistrationOpen, nil
	}
	return s.mode, nil
}

func (s *InMemoryRegistrationStore) SetRegistrationMode(mode RegistrationMode) error {
	if !isValidRegistrationMode(mode) {
		return ErrInvalidRegistrationMode
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mode = mode
	return nil
}

type InMemoryInviteStore struct {
	mu      sync.Mutex
	invites map[string]*Invite
}

func NewInMemoryInviteStore() *InMemoryInviteStore {
	return &InMemoryInviteStore{
		invites: make(map[string]*Invite),
	}
}

func (s *InMemoryInviteStore) CreateInvite(createdBy string, ttl time.Duration) (*Invite, error) {
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
	s.mu.Lock()
	s.invites[token] = invite
	s.mu.Unlock()
	return invite, nil
}

func (s *InMemoryInviteStore) GetInvite(token string) (*Invite, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	invite, ok := s.invites[token]
	if !ok {
		return nil, ErrInviteNotFound
	}
	cp := *invite
	return &cp, nil
}

func (s *InMemoryInviteStore) ConsumeInvite(token, usedBy string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	invite, ok := s.invites[token]
	if !ok {
		return ErrInviteNotFound
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
	return nil
}

func (s *InMemoryInviteStore) ListInvites() ([]*Invite, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	invites := make([]*Invite, 0, len(s.invites))
	for _, invite := range s.invites {
		cp := *invite
		invites = append(invites, &cp)
	}
	sort.Slice(invites, func(i, j int) bool {
		return invites[i].CreatedAt.After(invites[j].CreatedAt)
	})
	return invites, nil
}
