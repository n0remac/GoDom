package auth

import "time"

// UserRepo defines operations needed by auth handlers for user management.
type UserRepo interface {
	CreateUser(email, password string) (*User, error)
	GetByEmail(email string) (*User, error)
	GetByID(id string) (*User, error)
	VerifyPassword(email, password string) error
}

// SessionRepo defines session lifecycle operations.
type SessionRepo interface {
	Create(userID string, ttl time.Duration) (*Session, error)
	Get(id string) (*Session, bool)
	Delete(id string)
}
