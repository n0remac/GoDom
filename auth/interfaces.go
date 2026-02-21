package auth

import "time"

type RegistrationMode string

const (
	RegistrationOpen       RegistrationMode = "open"
	RegistrationInviteOnly RegistrationMode = "invite_only"
	RegistrationClosed     RegistrationMode = "closed"
)

type Invite struct {
	Token     string     `json:"token"`
	CreatedBy string     `json:"created_by"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	UsedBy    string     `json:"used_by,omitempty"`
}

func (i *Invite) IsUsed() bool {
	return i != nil && i.UsedAt != nil
}

func (i *Invite) IsExpired(now time.Time) bool {
	return i != nil && i.ExpiresAt != nil && now.After(*i.ExpiresAt)
}

// UserRepo defines operations needed by auth handlers for user management.
type UserRepo interface {
	CreateUser(email, password string) (*User, error)
	GetByEmail(email string) (*User, error)
	GetByID(id string) (*User, error)
	VerifyPassword(email, password string) error
	ListUsers() ([]*User, error)
	UpdateRole(id, role string) error
	DeleteUser(id string) error
	CountByRole(role string) (int, error)
}

// SessionRepo defines session lifecycle operations.
type SessionRepo interface {
	Create(userID string, ttl time.Duration) (*Session, error)
	Get(id string) (*Session, bool)
	Delete(id string)
}

// RegistrationRepo controls who can create new users.
type RegistrationRepo interface {
	GetRegistrationMode() (RegistrationMode, error)
	SetRegistrationMode(mode RegistrationMode) error
}

// InviteRepo manages one-time registration links.
type InviteRepo interface {
	CreateInvite(createdBy string, ttl time.Duration) (*Invite, error)
	GetInvite(token string) (*Invite, error)
	ConsumeInvite(token, usedBy string) error
	ListInvites() ([]*Invite, error)
}
