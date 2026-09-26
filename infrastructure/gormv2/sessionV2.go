package gormv2

import (
	"auxilia/domain/model"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"golang.org/x/crypto/bcrypt"
	"time"
)

var ErrUnauthenticated = errors.New("invalid credentials or expired session")

type Session struct {
	TokenHash string    `gorm:"type:char(64);primaryKey"`
	PlayerID  string    `gorm:"type:char(36);index"`
	ExpiresAt time.Time `gorm:"index"`
}

func (Session) TableName() string { return "battle_v2_sessions" }
func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
func (s *Store) Login(ctx context.Context, name, password string) (string, string, time.Time, error) {
	if name == "" || len(name) > 128 || password == "" || len(password) > 72 {
		return "", "", time.Time{}, ErrUnauthenticated
	}
	var user model.User
	if err := s.DB.WithContext(ctx).First(&user, "name = ?", name).Error; err != nil {
		return "", "", time.Time{}, ErrUnauthenticated
	}
	if bcrypt.CompareHashAndPassword([]byte(user.Hash), []byte(password)) != nil {
		return "", "", time.Time{}, ErrUnauthenticated
	}
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", time.Time{}, err
	}
	token := hex.EncodeToString(bytes)
	expires := time.Now().UTC().Add(24 * time.Hour)
	err := s.DB.WithContext(ctx).Create(&Session{TokenHash: tokenHash(token), PlayerID: user.ID.String(), ExpiresAt: expires}).Error
	return token, user.ID.String(), expires, err
}
func (s *Store) Authenticate(ctx context.Context, token string) (string, error) {
	if len(token) != 64 {
		return "", ErrUnauthenticated
	}
	var session Session
	if err := s.DB.WithContext(ctx).First(&session, "token_hash = ? AND expires_at > ?", tokenHash(token), time.Now()).Error; err != nil {
		return "", ErrUnauthenticated
	}
	return session.PlayerID, nil
}
