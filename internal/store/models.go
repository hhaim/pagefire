package store

import "time"

const (
	RoleAdmin = "admin"
	RoleUser  = "user"
)

type User struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Email          string     `json:"email"`
	Role           string     `json:"role"`
	Timezone       string     `json:"timezone"`
	AvatarURL      string     `json:"avatar_url,omitempty"`
	PasswordHash   string     `json:"-"`
	AuthProvider   *string    `json:"auth_provider,omitempty"`
	AuthProviderID *string    `json:"-"`
	IsActive       bool       `json:"is_active"`
	LastLogin      *time.Time `json:"last_login,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

type InviteToken struct {
	ID        string     `json:"id"`
	UserID    string     `json:"user_id"`
	TokenHash string     `json:"-"`
	ExpiresAt time.Time  `json:"expires_at"`
	UsedAt    *time.Time `json:"used_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}
