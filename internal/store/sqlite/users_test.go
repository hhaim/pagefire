package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/pagefire/pagefire/internal/store"
)

func seedUser(t *testing.T, users store.UserStore, name, email string) *store.User {
	t.Helper()
	u := &store.User{
		Name:      name,
		Email:     email,
		Role:      "admin",
		Timezone:  "America/New_York",
		AvatarURL: "https://example.com/avatar.png",
	}
	if err := users.Create(context.Background(), u); err != nil {
		t.Fatalf("seed user %q: %v", name, err)
	}
	return u
}

func TestUserCreateAndGetByID(t *testing.T) {
	s := newTestStore(t)
	users := s.Users()
	ctx := context.Background()

	u := &store.User{
		Name:      "Alice",
		Email:     "alice@example.com",
		Role:      "admin",
		Timezone:  "UTC",
		AvatarURL: "https://example.com/alice.png",
	}

	if err := users.Create(ctx, u); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if u.ID == "" {
		t.Fatal("expected ID to be assigned after Create")
	}

	got, err := users.Get(ctx, u.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if got.ID != u.ID {
		t.Errorf("ID = %q, want %q", got.ID, u.ID)
	}
	if got.Name != "Alice" {
		t.Errorf("Name = %q, want %q", got.Name, "Alice")
	}
	if got.Email != "alice@example.com" {
		t.Errorf("Email = %q, want %q", got.Email, "alice@example.com")
	}
	if got.Role != "admin" {
		t.Errorf("Role = %q, want %q", got.Role, "admin")
	}
	if got.Timezone != "UTC" {
		t.Errorf("Timezone = %q, want %q", got.Timezone, "UTC")
	}
	if got.AvatarURL != "https://example.com/alice.png" {
		t.Errorf("AvatarURL = %q, want %q", got.AvatarURL, "https://example.com/alice.png")
	}
	if got.CreatedAt.IsZero() {
		t.Error("CreatedAt should not be zero")
	}
}

func TestUserGetByEmail(t *testing.T) {
	s := newTestStore(t)
	users := s.Users()
	ctx := context.Background()

	u := seedUser(t, users, "Bob", "bob@example.com")

	got, err := users.GetByEmail(ctx, "bob@example.com")
	if err != nil {
		t.Fatalf("GetByEmail: %v", err)
	}
	if got.ID != u.ID {
		t.Errorf("ID = %q, want %q", got.ID, u.ID)
	}
	if got.Name != "Bob" {
		t.Errorf("Name = %q, want %q", got.Name, "Bob")
	}
	if got.Email != "bob@example.com" {
		t.Errorf("Email = %q, want %q", got.Email, "bob@example.com")
	}
}

func TestUserDuplicateEmail(t *testing.T) {
	s := newTestStore(t)
	users := s.Users()
	ctx := context.Background()

	seedUser(t, users, "First", "dup@example.com")

	second := &store.User{
		Name:     "Second",
		Email:    "dup@example.com",
		Role:     "user",
		Timezone: "UTC",
	}
	err := users.Create(ctx, second)
	if err == nil {
		t.Fatal("expected error for duplicate email, got nil")
	}
}

func TestUserGetNotFound(t *testing.T) {
	s := newTestStore(t)
	users := s.Users()
	ctx := context.Background()

	tests := []struct {
		name string
		fn   func() error
	}{
		{
			name: "Get by non-existent ID",
			fn: func() error {
				_, err := users.Get(ctx, "no-such-id")
				return err
			},
		},
		{
			name: "GetByEmail non-existent",
			fn: func() error {
				_, err := users.GetByEmail(ctx, "nobody@example.com")
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			if !errors.Is(err, store.ErrNotFound) {
				t.Errorf("err = %v, want store.ErrNotFound", err)
			}
		})
	}
}

func TestUserList(t *testing.T) {
	s := newTestStore(t)
	users := s.Users()
	ctx := context.Background()

	// List should return empty initially.
	list, err := users.List(ctx)
	if err != nil {
		t.Fatalf("List (empty): %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 users, got %d", len(list))
	}

	// Insert users out of alphabetical order to verify ORDER BY name.
	seedUser(t, users, "Charlie", "charlie@example.com")
	seedUser(t, users, "Alice", "alice@example.com")
	seedUser(t, users, "Bob", "bob@example.com")

	list, err = users.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 users, got %d", len(list))
	}

	// Verify alphabetical ordering.
	wantNames := []string{"Alice", "Bob", "Charlie"}
	for i, want := range wantNames {
		if list[i].Name != want {
			t.Errorf("list[%d].Name = %q, want %q", i, list[i].Name, want)
		}
	}
}

func TestUserUpdate(t *testing.T) {
	s := newTestStore(t)
	users := s.Users()
	ctx := context.Background()

	u := seedUser(t, users, "Original", "orig@example.com")

	u.Name = "Updated"
	u.Email = "updated@example.com"
	u.Role = "user"
	u.Timezone = "Europe/London"
	u.AvatarURL = "https://example.com/new.png"

	if err := users.Update(ctx, u); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := users.Get(ctx, u.ID)
	if err != nil {
		t.Fatalf("Get after Update: %v", err)
	}

	if got.Name != "Updated" {
		t.Errorf("Name = %q, want %q", got.Name, "Updated")
	}
	if got.Email != "updated@example.com" {
		t.Errorf("Email = %q, want %q", got.Email, "updated@example.com")
	}
	if got.Role != "user" {
		t.Errorf("Role = %q, want %q", got.Role, "user")
	}
	if got.Timezone != "Europe/London" {
		t.Errorf("Timezone = %q, want %q", got.Timezone, "Europe/London")
	}
	if got.AvatarURL != "https://example.com/new.png" {
		t.Errorf("AvatarURL = %q, want %q", got.AvatarURL, "https://example.com/new.png")
	}
}

func TestUserUpdateNotFound(t *testing.T) {
	s := newTestStore(t)
	users := s.Users()
	ctx := context.Background()

	err := users.Update(ctx, &store.User{ID: "nonexistent", Name: "X", Email: "x@x.com"})
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Update non-existent: err = %v, want store.ErrNotFound", err)
	}
}

func TestUserDelete(t *testing.T) {
	s := newTestStore(t)
	users := s.Users()
	ctx := context.Background()

	u := seedUser(t, users, "ToDelete", "delete@example.com")

	if err := users.Delete(ctx, u.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := users.Get(ctx, u.ID)
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Get after Delete: err = %v, want store.ErrNotFound", err)
	}
}

func TestUserDeleteNotFound(t *testing.T) {
	s := newTestStore(t)
	users := s.Users()
	ctx := context.Background()

	err := users.Delete(ctx, "no-such-id")
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("Delete non-existent: err = %v, want store.ErrNotFound", err)
	}
}
