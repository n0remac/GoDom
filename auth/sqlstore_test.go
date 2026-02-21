package auth

import (
	"errors"
	"testing"
	"time"

	"github.com/n0remac/GoDom/database"
)

func newTestSQLiteStore(t *testing.T) *SQLiteStore {
	t.Helper()
	ds, err := database.NewSQLiteStoreFromDSN(":memory:")
	if err != nil {
		t.Fatalf("open sqlite store: %v", err)
	}
	t.Cleanup(func() {
		_ = ds.Close()
	})
	return NewSQLiteStore(ds)
}

func TestEnsureAdminBootstrap(t *testing.T) {
	s := newTestSQLiteStore(t)

	created, err := s.EnsureAdmin("admin@example.com", "secret")
	if err != nil {
		t.Fatalf("EnsureAdmin create: %v", err)
	}
	if !created {
		t.Fatalf("expected admin to be created")
	}

	user, err := s.GetByEmail("admin@example.com")
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	if user.Role != RoleAdmin {
		t.Fatalf("expected role %q, got %q", RoleAdmin, user.Role)
	}

	admins, err := s.CountByRole(RoleAdmin)
	if err != nil {
		t.Fatalf("CountByRole: %v", err)
	}
	if admins != 1 {
		t.Fatalf("expected 1 admin, got %d", admins)
	}
}

func TestRegistrationModeAndInviteLifecycle(t *testing.T) {
	s := newTestSQLiteStore(t)

	mode, err := s.GetRegistrationMode()
	if err != nil {
		t.Fatalf("GetRegistrationMode default: %v", err)
	}
	if mode != RegistrationOpen {
		t.Fatalf("expected default mode %q, got %q", RegistrationOpen, mode)
	}

	if err := s.SetRegistrationMode(RegistrationInviteOnly); err != nil {
		t.Fatalf("SetRegistrationMode: %v", err)
	}
	mode, err = s.GetRegistrationMode()
	if err != nil {
		t.Fatalf("GetRegistrationMode set value: %v", err)
	}
	if mode != RegistrationInviteOnly {
		t.Fatalf("expected mode %q, got %q", RegistrationInviteOnly, mode)
	}

	invite, err := s.CreateInvite("admin-id", time.Hour)
	if err != nil {
		t.Fatalf("CreateInvite: %v", err)
	}
	if invite.Token == "" {
		t.Fatalf("expected invite token")
	}
	if invite.UsedAt != nil {
		t.Fatalf("new invite should not be used")
	}

	if err := s.ConsumeInvite(invite.Token, "user-1"); err != nil {
		t.Fatalf("ConsumeInvite first use: %v", err)
	}
	if err := s.ConsumeInvite(invite.Token, "user-2"); !errors.Is(err, ErrInviteUsed) {
		t.Fatalf("expected ErrInviteUsed on second consume, got %v", err)
	}
}
