package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const SessionTTL = 7 * 24 * time.Hour

var ErrInvalidLogin = errors.New("账号或密码不正确")

func (s *Store) CountUsers() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func (s *Store) EnsureAuthUsers(seeds []AuthUserSeed) (usedDefault bool, err error) {
	clean := make([]AuthUserSeed, 0, len(seeds))
	for _, seed := range seeds {
		name := strings.TrimSpace(seed.Username)
		if name == "" || seed.Password == "" {
			continue
		}
		clean = append(clean, AuthUserSeed{Username: name, Password: seed.Password})
	}
	if len(clean) == 0 {
		n, err := s.CountUsers()
		if err != nil {
			return false, err
		}
		if n > 0 {
			return false, nil
		}
		clean = []AuthUserSeed{{Username: "admin", Password: "ocrshow"}}
		usedDefault = true
	}
	for _, seed := range clean {
		if err := s.upsertUser(seed.Username, seed.Password); err != nil {
			return false, err
		}
	}
	return usedDefault, nil
}

func (s *Store) upsertUser(username, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	now := NowISO()
	_, err = s.db.Exec(`
INSERT INTO users (username, password_hash, created_at) VALUES (?, ?, ?)
ON CONFLICT(username) DO UPDATE SET password_hash = excluded.password_hash
`, username, string(hash), now)
	return err
}

func (s *Store) Authenticate(username, password string) (*User, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return nil, ErrInvalidLogin
	}
	var u User
	var hash string
	err := s.db.QueryRow(
		`SELECT id, username, password_hash FROM users WHERE username = ?`,
		username,
	).Scan(&u.ID, &u.Username, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		_ = bcrypt.CompareHashAndPassword([]byte("$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy"), []byte(password))
		return nil, ErrInvalidLogin
	}
	if err != nil {
		return nil, err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return nil, ErrInvalidLogin
	}
	return &u, nil
}

func (s *Store) CreateSession(userID int64) (token string, expires time.Time, err error) {
	raw := make([]byte, 32)
	if _, err = io.ReadFull(rand.Reader, raw); err != nil {
		return "", time.Time{}, err
	}
	token = hex.EncodeToString(raw)
	now := time.Now()
	expires = now.Add(SessionTTL)
	_, err = s.db.Exec(
		`INSERT INTO sessions (token, user_id, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		token, userID, expires.UTC().Format(time.RFC3339), now.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

func (s *Store) UserBySession(token string) (*User, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, sql.ErrNoRows
	}
	_, _ = s.db.Exec(`DELETE FROM sessions WHERE expires_at <= ?`, time.Now().UTC().Format(time.RFC3339))
	var u User
	err := s.db.QueryRow(`
SELECT u.id, u.username
FROM sessions s
JOIN users u ON u.id = s.user_id
WHERE s.token = ? AND s.expires_at > ?
`, token, time.Now().UTC().Format(time.RFC3339)).Scan(&u.ID, &u.Username)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *Store) DeleteSession(token string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token = ?`, token)
	return err
}
