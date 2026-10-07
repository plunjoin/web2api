package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Cookie sessions are tied to credentials so replacing an account cannot revive
// a stale session, including an old refresh completing after replacement.
func (s *Store) LoadCookieSession(id int64, psid, psidts string) ([]byte, error) {
	var data []byte
	err := s.db.QueryRow(`SELECT c.data FROM cookie_sessions c JOIN accounts a ON a.id=c.account_id
		WHERE a.id=? AND a.engine='a' AND json_extract(a.credentials,'$.psid')=?
		AND coalesce(json_extract(a.credentials,'$.psidts'),'')=?`, id, psid, psidts).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return data, err
}

// ImportLegacyCookies reads old cache files once; it never creates or deletes
// that directory. Only files belonging to known SQLite accounts are imported.
func (s *Store) ImportLegacyCookies(directory string) (int, error) {
	accounts, err := s.ListAccounts()
	if err != nil {
		return 0, err
	}
	root, err := filepath.Abs(directory)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, account := range accounts {
		if account.Engine != "a" {
			continue
		}
		var credentials struct {
			PSID   string `json:"psid"`
			PSIDTS string `json:"psidts"`
		}
		if json.Unmarshal([]byte(account.Credentials), &credentials) != nil || credentials.PSID == "" {
			continue
		}
		name := ".cached_cookies_" + credentials.PSID + ".json"
		if strings.ContainsAny(name, "/\\") {
			return count, fmt.Errorf("invalid legacy cookie filename for account %d", account.ID)
		}
		data, err := os.ReadFile(filepath.Join(root, name))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return count, fmt.Errorf("read legacy cookie session for account %d: %w", account.ID, err)
		}
		var cookies []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		}
		if err := json.Unmarshal(data, &cookies); err != nil {
			return count, fmt.Errorf("invalid cookie JSON for account %d", account.ID)
		}
		matches := false
		for _, cookie := range cookies {
			if cookie.Name == "__Secure-1PSID" && cookie.Value == credentials.PSID {
				matches = true
			}
		}
		if !matches {
			return count, fmt.Errorf("legacy session does not match account %d", account.ID)
		}
		if err := s.SaveCookieSession(account.ID, credentials.PSID, credentials.PSIDTS, data); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

func (s *Store) SaveCookieSession(id int64, psid, psidts string, data []byte) error {
	_, err := s.db.Exec(`INSERT INTO cookie_sessions(account_id,data,updated_at)
		SELECT id,?,? FROM accounts WHERE id=? AND engine='a' AND json_extract(credentials,'$.psid')=?
		AND coalesce(json_extract(credentials,'$.psidts'),'')=?
		ON CONFLICT(account_id) DO UPDATE SET data=excluded.data,updated_at=excluded.updated_at`,
		data, now(), id, psid, psidts)
	return err
}

func (s *Store) DeleteCookieSession(id int64, psid, psidts string) error {
	_, err := s.db.Exec(`DELETE FROM cookie_sessions WHERE account_id IN
		(SELECT id FROM accounts WHERE id=? AND engine='a' AND json_extract(credentials,'$.psid')=?
		AND coalesce(json_extract(credentials,'$.psidts'),'')=?)`, id, psid, psidts)
	return err
}
