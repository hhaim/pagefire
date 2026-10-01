package homealerts

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"
)

type IngestionKeyInfo struct {
	Configured  bool   `json:"configured"`
	Recoverable bool   `json:"recoverable"`
	Prefix      string `json:"prefix,omitempty"`
	CreatedAt   int64  `json:"created_at,omitempty"`
}

func (s *Service) IngestionKey(ctx context.Context) (IngestionKeyInfo, error) {
	var info IngestionKeyInfo
	var encrypted string
	err := s.db.QueryRowContext(ctx, `SELECT prefix,created_at,encrypted_token FROM home_ingestion_key WHERE id='default'`).Scan(&info.Prefix, &info.CreatedAt, &encrypted)
	if err == sql.ErrNoRows {
		return info, nil
	}
	if err != nil {
		return info, err
	}
	info.Configured = true
	info.Recoverable = encrypted != ""
	return info, nil
}

func (s *Service) RotateIngestionKey(ctx context.Context, ownerUserID string) (string, error) {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", err
	}
	token := "pfe_" + hex.EncodeToString(secret)
	hash := sha256.Sum256([]byte(token))
	encrypted, err := s.encrypt(token)
	if err != nil {
		return "", err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO home_ingestion_key(id,owner_user_id,token_hash,prefix,created_at,encrypted_token) VALUES('default',?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET token_hash=excluded.token_hash,prefix=excluded.prefix,created_at=excluded.created_at,encrypted_token=excluded.encrypted_token`, ownerUserID, hash[:], token[:12], time.Now().UTC().Unix(), encrypted)
	if err != nil {
		return "", err
	}
	return token, nil
}

func (s *Service) RememberIngestionKey(ctx context.Context, token string) error {
	if _, err := s.ValidateIngestionKey(ctx, token); err != nil {
		return fmt.Errorf("%w: key does not match the active ingestion key", ErrInvalid)
	}
	encrypted, err := s.encrypt(token)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE home_ingestion_key SET encrypted_token=? WHERE id='default'`, encrypted)
	return err
}

func (s *Service) RevealIngestionKey(ctx context.Context) (string, error) {
	var encrypted string
	err := s.db.QueryRowContext(ctx, `SELECT encrypted_token FROM home_ingestion_key WHERE id='default'`).Scan(&encrypted)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if encrypted == "" {
		return "", ErrNotFound
	}
	return s.decrypt(encrypted)
}

func (s *Service) ValidateIngestionKey(ctx context.Context, token string) (string, error) {
	if len(token) != 68 || token[:4] != "pfe_" {
		return "", ErrNotFound
	}
	var owner string
	var expected []byte
	err := s.db.QueryRowContext(ctx, `SELECT owner_user_id,token_hash FROM home_ingestion_key WHERE id='default'`).Scan(&owner, &expected)
	if err == sql.ErrNoRows {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	actual := sha256.Sum256([]byte(token))
	if subtle.ConstantTimeCompare(actual[:], expected) != 1 {
		return "", ErrNotFound
	}
	return owner, nil
}

func (s *Service) RevokeIngestionKey(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM home_ingestion_key WHERE id='default'`)
	return err
}
