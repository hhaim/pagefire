package store

import (
	"context"
	"errors"
)

var (
	ErrNotFound     = errors.New("not found")
	ErrDuplicateKey = errors.New("duplicate key")
	ErrConflict     = errors.New("conflict")
)

// Store holds the account repository and database lifecycle.
type Store interface {
	Users() UserStore
	Ping(ctx context.Context) error
	Migrate(ctx context.Context) error
	Close() error
}

// UserStore manages account, invite, and API credentials.
type UserStore interface {
	Create(ctx context.Context, u *User) error
	Get(ctx context.Context, id string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	List(ctx context.Context) ([]User, error)
	Update(ctx context.Context, u *User) error
	Delete(ctx context.Context, id string) error
	SetPassword(ctx context.Context, id string, passwordHash string) error
	SetLastLogin(ctx context.Context, id string) error
	CountUsers(ctx context.Context) (int, error)

	CreateInviteToken(ctx context.Context, t *InviteToken) error
	GetInviteTokenByHash(ctx context.Context, tokenHash string) (*InviteToken, error)
	UseInviteToken(ctx context.Context, id string) error
}
