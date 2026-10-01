package db

import (
	"context"
	"database/sql"
	"time"
)

type Account struct {
	UID   int64
	Email string
	UName string
	IsNew bool
}

func (s *Store) UpsertAccount(ctx context.Context, email, defaultUName string) (Account, error) {
	var acc Account
	row := s.conn.QueryRowContext(ctx,
		`SELECT uid, email, u_name FROM accounts WHERE email = $1`, email)
	err := row.Scan(&acc.UID, &acc.Email, &acc.UName)
	if err == nil {
		return acc, nil
	}
	if err != sql.ErrNoRows {
		return Account{}, err
	}

	row = s.conn.QueryRowContext(ctx,
		`INSERT INTO accounts (email, u_name) VALUES ($1, $2)
		 ON CONFLICT (email) DO UPDATE SET email = EXCLUDED.email
		 RETURNING uid, email, u_name`, email, defaultUName)
	if err := row.Scan(&acc.UID, &acc.Email, &acc.UName); err != nil {
		return Account{}, err
	}
	acc.IsNew = true
	return acc, nil
}

func (s *Store) SaveSession(ctx context.Context, accessKey string, uid int64, expiresAt time.Time) error {
	_, err := s.conn.ExecContext(ctx,
		`INSERT INTO sessions (access_key, uid, expires_at) VALUES ($1, $2, $3)`,
		accessKey, uid, expiresAt)
	return err
}

type SessionLookup struct {
	UID   int64
	Email string
	UName string
}

func (s *Store) LookupSession(ctx context.Context, accessKey string) (SessionLookup, bool, error) {
	var out SessionLookup
	row := s.conn.QueryRowContext(ctx,
		`SELECT a.uid, a.email, a.u_name
		 FROM sessions s JOIN accounts a ON a.uid = s.uid
		 WHERE s.access_key = $1 AND s.expires_at > now()`, accessKey)
	err := row.Scan(&out.UID, &out.Email, &out.UName)
	if err == sql.ErrNoRows {
		return SessionLookup{}, false, nil
	}
	if err != nil {
		return SessionLookup{}, false, err
	}
	return out, true, nil
}