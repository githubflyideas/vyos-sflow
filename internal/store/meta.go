package store

// Meta reads a value from the meta table ("" when absent).
func (s *Store) Meta(k string) string {
	var v string
	s.DB.QueryRow(`SELECT v FROM meta WHERE k = ?`, k).Scan(&v)
	return v
}

// SetMeta stores a value in the meta table.
func (s *Store) SetMeta(k, v string) error {
	_, err := s.DB.Exec(`INSERT INTO meta VALUES (?, ?) ON CONFLICT (k) DO UPDATE SET v = excluded.v`, k, v)
	return err
}
