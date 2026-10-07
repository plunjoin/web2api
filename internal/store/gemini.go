package store

import "time"

func (s *Store) SaveGeminiUploadURL(id, url string, expires time.Time) error {
	if _, err := s.db.Exec(`DELETE FROM gemini_upload_sessions WHERE expires_at <= ?`, now()); err != nil {
		return err
	}
	_, err := s.db.Exec(`INSERT INTO gemini_upload_sessions(id,url,expires_at) VALUES(?,?,?)`, id, url, expires.Unix())
	return err
}

func (s *Store) GeminiUploadURL(id string) (string, error) {
	var url string
	err := s.db.QueryRow(`SELECT url FROM gemini_upload_sessions WHERE id=? AND expires_at>?`, id, now()).Scan(&url)
	return url, err
}
