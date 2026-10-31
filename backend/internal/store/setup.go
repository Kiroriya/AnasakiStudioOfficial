package store

import "database/sql"

// setupTables are the tables the first-run setup adds rows to.
var setupTables = []string{"users", "posts", "revisions", "terms", "menus", "components", "component_revisions", "media"}

// SetupMark records the highest id of every setup table (RollbackSetup).
type SetupMark map[string]int64

// MarkSetup notes where the tables stand before the first-run setup.
func (s *Store) MarkSetup() (SetupMark, error) {
	m := SetupMark{}
	for _, t := range setupTables {
		var id sql.NullInt64
		if err := s.DB.QueryRow(`SELECT MAX(id) FROM ` + t).Scan(&id); err != nil {
			return nil, err
		}
		m[t] = id.Int64
	}
	return m, nil
}

// RollbackSetup removes every row added since m (the administrator and the
// starter content of a setup that failed half-way), so the setup can run
// again. It returns the URLs of the removed media files.
func (s *Store) RollbackSetup(m SetupMark) ([]string, error) {
	var urls []string
	err := s.inTx(func(tx *sql.Tx) error {
		rows, err := tx.Query(`SELECT url FROM media WHERE id > ?`, m["media"])
		if err != nil {
			return err
		}
		for rows.Next() {
			var u string
			if err := rows.Scan(&u); err != nil {
				rows.Close()
				return err
			}
			urls = append(urls, u)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM post_terms WHERE post_id > ? OR term_id > ?`, m["posts"], m["terms"]); err != nil {
			return err
		}
		for _, t := range setupTables {
			if _, err := tx.Exec(`DELETE FROM `+t+` WHERE id > ?`, m[t]); err != nil {
				return err
			}
		}
		return nil
	})
	return urls, err
}
