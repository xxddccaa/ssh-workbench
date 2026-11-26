package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	_ "modernc.org/sqlite"

	"sshworkbench/internal/connections"
	"sshworkbench/internal/fault"
	"sshworkbench/internal/hosts"
)

type Store struct {
	database *sql.DB
	filename string
}

func Open(directory string) (*Store, error) {
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	info, err := os.Stat(directory)
	if err != nil {
		return nil, err
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		return nil, fmt.Errorf("data directory must be private (0700): %s", directory)
	}
	filename := filepath.Join(directory, "workbench.db")
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	database, err := sql.Open("sqlite", filename)
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(1)
	store := &Store{database: database, filename: filename}
	if err := store.migrate(); err != nil {
		database.Close()
		return nil, err
	}
	return store, nil
}

const schemaVersion = 2

var migrations = []string{
	1: `
		CREATE TABLE IF NOT EXISTS hosts (id TEXT PRIMARY KEY, name TEXT NOT NULL, document TEXT NOT NULL);
		CREATE TABLE IF NOT EXISTS known_keys (address TEXT PRIMARY KEY, algorithm TEXT NOT NULL, fingerprint TEXT NOT NULL, public_key BLOB NOT NULL, trusted_at TEXT NOT NULL);
		CREATE TABLE IF NOT EXISTS settings (name TEXT PRIMARY KEY, value TEXT NOT NULL);
	`,
	2: `
		CREATE TABLE IF NOT EXISTS credentials (id TEXT PRIMARY KEY, label TEXT NOT NULL, kind TEXT NOT NULL, secret BLOB NOT NULL, created_at TEXT NOT NULL);
		CREATE TABLE IF NOT EXISTS tunnels (id TEXT PRIMARY KEY, host_id TEXT NOT NULL, document TEXT NOT NULL);
		CREATE TABLE IF NOT EXISTS snippets (id TEXT PRIMARY KEY, name TEXT NOT NULL, document TEXT NOT NULL);
	`,
}

// migrate upgrades the schema in one transaction. An existing database is
// first copied with VACUUM INTO, so a failed or unwanted upgrade can be
// rolled back by hand. The copy contains sealed credentials and stays in
// the private data directory.
func (store *Store) migrate() error {
	if _, err := store.database.Exec("PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000; PRAGMA foreign_keys=ON;"); err != nil {
		return err
	}
	var version int
	if err := store.database.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version > schemaVersion {
		return errors.New("database was created by a newer version")
	}
	if version == schemaVersion {
		return nil
	}
	if version > 0 {
		backup := store.filename + fmt.Sprintf(".before-v%d.bak", schemaVersion)
		os.Remove(backup)
		if _, err := store.database.Exec("VACUUM INTO ?", backup); err != nil {
			return fmt.Errorf("back up database before upgrade: %w", err)
		}
		os.Chmod(backup, 0600)
	}
	transaction, err := store.database.Begin()
	if err != nil {
		return err
	}
	defer transaction.Rollback()
	for next := version + 1; next <= schemaVersion; next++ {
		if _, err := transaction.Exec(migrations[next]); err != nil {
			return err
		}
	}
	if _, err := transaction.Exec(fmt.Sprintf("PRAGMA user_version=%d", schemaVersion)); err != nil {
		return err
	}
	return transaction.Commit()
}

func (store *Store) Close() error {
	return store.database.Close()
}

func (store *Store) ListHosts(ctx context.Context) ([]hosts.Profile, error) {
	rows, err := store.database.QueryContext(ctx, "SELECT document FROM hosts ORDER BY name COLLATE NOCASE, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	profiles := make([]hosts.Profile, 0)
	for rows.Next() {
		var document string
		var profile hosts.Profile
		if err := rows.Scan(&document); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(document), &profile); err != nil {
			return nil, err
		}
		profiles = append(profiles, profile)
	}
	return profiles, rows.Err()
}

func (store *Store) GetHost(ctx context.Context, hostID string) (hosts.Profile, error) {
	var document string
	var profile hosts.Profile
	err := store.database.QueryRowContext(ctx, "SELECT document FROM hosts WHERE id=?", hostID).Scan(&document)
	if errors.Is(err, sql.ErrNoRows) {
		return profile, fault.New(404, "host_not_found", "主机配置不存在。")
	}
	if err != nil {
		return profile, err
	}
	err = json.Unmarshal([]byte(document), &profile)
	return profile, err
}

func (store *Store) PutHost(ctx context.Context, profile hosts.Profile) error {
	document, err := json.Marshal(profile)
	if err != nil {
		return err
	}
	_, err = store.database.ExecContext(ctx, "INSERT INTO hosts(id,name,document) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name, document=excluded.document", profile.ID, profile.Name, string(document))
	return err
}

func (store *Store) DeleteHost(ctx context.Context, hostID string) error {
	result, err := store.database.ExecContext(ctx, "DELETE FROM hosts WHERE id=?", hostID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count == 0 {
		return fault.New(404, "host_not_found", "主机配置不存在。")
	}
	return err
}

func (store *Store) GetKnownKey(ctx context.Context, address string) (*connections.KnownKey, error) {
	var key connections.KnownKey
	var trustedAt string
	err := store.database.QueryRowContext(ctx, "SELECT address,algorithm,fingerprint,public_key,trusted_at FROM known_keys WHERE address=?", address).Scan(&key.Address, &key.Algorithm, &key.Fingerprint, &key.PublicKey, &trustedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	key.TrustedAt, err = time.Parse(time.RFC3339, trustedAt)
	if err != nil {
		return nil, err
	}
	return &key, nil
}

func (store *Store) AddKnownKey(ctx context.Context, key connections.KnownKey) error {
	_, err := store.database.ExecContext(ctx, "INSERT INTO known_keys(address,algorithm,fingerprint,public_key,trusted_at) VALUES(?,?,?,?,?) ON CONFLICT(address) DO NOTHING", key.Address, key.Algorithm, key.Fingerprint, key.PublicKey, key.TrustedAt.Format("2006-01-02T15:04:05Z07:00"))
	if err != nil {
		return err
	}
	known, err := store.GetKnownKey(ctx, key.Address)
	if err != nil {
		return err
	}
	if known == nil || !bytes.Equal(known.PublicKey, key.PublicKey) {
		return fault.New(409, "host_key_changed", "另一条信任记录已存在，不能覆盖。")
	}
	return nil
}

func (store *Store) GetSetting(ctx context.Context, name string) (string, error) {
	var value string
	err := store.database.QueryRowContext(ctx, "SELECT value FROM settings WHERE name=?", name).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return value, err
}

func (store *Store) PutSetting(ctx context.Context, name, value string) error {
	_, err := store.database.ExecContext(ctx, "INSERT INTO settings(name,value) VALUES(?,?) ON CONFLICT(name) DO UPDATE SET value=excluded.value", name, value)
	return err
}

func (store *Store) DeleteSetting(ctx context.Context, name string) error {
	_, err := store.database.ExecContext(ctx, "DELETE FROM settings WHERE name=?", name)
	return err
}
