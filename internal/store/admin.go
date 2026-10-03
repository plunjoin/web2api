package store

import (
	"database/sql"
	"errors"
)

// AdminSettings is a singleton, created only through first-run setup.
type AdminSettings struct {
	Email        string `json:"email"`
	Nickname     string `json:"nickname"`
	PasswordHash string `json:"-"`
	RedisURL     string `json:"-"`
	JWTSecret    string `json:"-"`
}

var ErrInitialized = errors.New("系统已经初始化")

func (s *Store) AdminSettings() (*AdminSettings, error) {
	v := &AdminSettings{}
	err := s.db.QueryRow(`SELECT email,nickname,password_hash,redis_url,jwt_secret FROM admin_settings WHERE id=1`).Scan(&v.Email, &v.Nickname, &v.PasswordHash, &v.RedisURL, &v.JWTSecret)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return v, err
}

func (s *Store) InitializeAdmin(v *AdminSettings) error {
	res, err := s.db.Exec(`INSERT INTO admin_settings(id,email,nickname,password_hash,redis_url,jwt_secret) VALUES(1,?,?,?,?,?) ON CONFLICT(id) DO NOTHING`, v.Email, v.Nickname, v.PasswordHash, v.RedisURL, v.JWTSecret)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err == nil && n == 0 {
		return ErrInitialized
	}
	return err
}
