package auth

import (
	"errors"
	"testing"
)

func TestInMemoryStoreEmailNormalization(t *testing.T) {
	s := NewInMemoryUserStore()

	created, err := s.CreateUser("  User@Example.COM  ", "secret")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if created.Email != "user@example.com" {
		t.Fatalf("expected normalized email %q, got %q", "user@example.com", created.Email)
	}

	got, err := s.GetByEmail("USER@EXAMPLE.COM")
	if err != nil {
		t.Fatalf("GetByEmail mixed-case: %v", err)
	}
	if got.ID != created.ID {
		t.Fatalf("expected user ID %q, got %q", created.ID, got.ID)
	}

	if err := s.VerifyPassword("UsEr@Example.cOm", "secret"); err != nil {
		t.Fatalf("VerifyPassword mixed-case: %v", err)
	}

	_, err = s.CreateUser("user@example.com", "another-secret")
	if !errors.Is(err, ErrUserExists) {
		t.Fatalf("expected ErrUserExists for case-insensitive duplicate, got %v", err)
	}
}
