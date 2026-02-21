package auth

import (
	"context"
	"encoding/json"
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

func TestSQLiteStoreEmailNormalization(t *testing.T) {
	s := newTestSQLiteStore(t)

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

func TestSQLiteStoreGetByEmailMigratesLegacyMixedCaseRecord(t *testing.T) {
	s := newTestSQLiteStore(t)
	ctx := context.Background()

	legacy := &User{
		ID:           "legacy-user",
		Email:        "Legacy@Example.COM",
		PasswordHash: []byte("hash"),
		Role:         RoleMember,
		CreatedAt:    time.Now(),
	}
	userDoc, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal legacy user: %v", err)
	}
	if err := s.ds.Put(ctx, userKey(legacy.ID), userDoc); err != nil {
		t.Fatalf("put legacy user: %v", err)
	}
	legacyIndex, err := json.Marshal(map[string]string{"id": legacy.ID})
	if err != nil {
		t.Fatalf("marshal legacy index: %v", err)
	}
	if err := s.ds.Put(ctx, emailKey(legacy.Email), legacyIndex); err != nil {
		t.Fatalf("put legacy index: %v", err)
	}

	got, err := s.GetByEmail("legacy@example.com")
	if err != nil {
		t.Fatalf("GetByEmail legacy mixed-case: %v", err)
	}
	if got.Email != "legacy@example.com" {
		t.Fatalf("expected normalized email %q, got %q", "legacy@example.com", got.Email)
	}

	normalizedIndex, err := s.ds.Get(ctx, emailKey("legacy@example.com"))
	if err != nil {
		t.Fatalf("get normalized index: %v", err)
	}
	if normalizedIndex == nil {
		t.Fatalf("expected normalized email index to be created")
	}

	oldIndex, err := s.ds.Get(ctx, emailKey("Legacy@Example.COM"))
	if err != nil {
		t.Fatalf("get old index: %v", err)
	}
	if oldIndex != nil {
		t.Fatalf("expected old mixed-case index to be removed")
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
