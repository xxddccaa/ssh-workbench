package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"sshworkbench/internal/credentials"
	"sshworkbench/internal/fault"
	"sshworkbench/internal/snippets"
	"sshworkbench/internal/tunnels"
)

func (store *Store) ListCredentials(ctx context.Context) ([]credentials.Sealed, error) {
	rows, err := store.database.QueryContext(ctx, "SELECT id,label,kind,secret,created_at FROM credentials ORDER BY label COLLATE NOCASE, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]credentials.Sealed, 0)
	for rows.Next() {
		item, err := scanCredential(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

type scanner interface{ Scan(...any) error }

func scanCredential(row scanner) (credentials.Sealed, error) {
	var item credentials.Sealed
	var created string
	if err := row.Scan(&item.ID, &item.Label, &item.Kind, &item.Secret, &created); err != nil {
		return item, err
	}
	parsed, err := time.Parse(time.RFC3339Nano, created)
	item.CreatedAt = parsed
	return item, err
}

func (store *Store) GetCredential(ctx context.Context, identifier string) (credentials.Sealed, error) {
	item, err := scanCredential(store.database.QueryRowContext(ctx, "SELECT id,label,kind,secret,created_at FROM credentials WHERE id=?", identifier))
	if errors.Is(err, sql.ErrNoRows) {
		return item, fault.New(404, "credential_not_found", "保存的凭据不存在，可能已被删除。")
	}
	return item, err
}

func (store *Store) PutCredential(ctx context.Context, item credentials.Sealed) error {
	_, err := store.database.ExecContext(ctx, "INSERT INTO credentials(id,label,kind,secret,created_at) VALUES(?,?,?,?,?)", item.ID, item.Label, item.Kind, item.Secret, item.CreatedAt.Format(time.RFC3339Nano))
	return err
}

func (store *Store) DeleteCredential(ctx context.Context, identifier string) error {
	return store.deleteRow(ctx, "DELETE FROM credentials WHERE id=?", identifier, fault.New(404, "credential_not_found", "保存的凭据不存在，可能已被删除。"))
}

func (store *Store) DeleteAllCredentials(ctx context.Context) error {
	_, err := store.database.ExecContext(ctx, "DELETE FROM credentials")
	return err
}

func (store *Store) deleteRow(ctx context.Context, statement, identifier string, missing error) error {
	result, err := store.database.ExecContext(ctx, statement, identifier)
	if err != nil {
		return err
	}
	if count, err := result.RowsAffected(); err == nil && count == 0 {
		return missing
	}
	return err
}

func listDocuments[Item any](ctx context.Context, store *Store, statement string) ([]Item, error) {
	rows, err := store.database.QueryContext(ctx, statement)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Item, 0)
	for rows.Next() {
		var document string
		var item Item
		if err := rows.Scan(&document); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(document), &item); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func getDocument[Item any](ctx context.Context, store *Store, statement, identifier string, missing error) (Item, error) {
	var document string
	var item Item
	err := store.database.QueryRowContext(ctx, statement, identifier).Scan(&document)
	if errors.Is(err, sql.ErrNoRows) {
		return item, missing
	}
	if err != nil {
		return item, err
	}
	return item, json.Unmarshal([]byte(document), &item)
}

func (store *Store) ListTunnels(ctx context.Context) ([]tunnels.Spec, error) {
	return listDocuments[tunnels.Spec](ctx, store, "SELECT document FROM tunnels ORDER BY id")
}

func (store *Store) GetTunnel(ctx context.Context, identifier string) (tunnels.Spec, error) {
	return getDocument[tunnels.Spec](ctx, store, "SELECT document FROM tunnels WHERE id=?", identifier, fault.New(404, "tunnel_not_found", "隧道不存在。"))
}

func (store *Store) PutTunnel(ctx context.Context, spec tunnels.Spec) error {
	document, err := json.Marshal(spec)
	if err != nil {
		return err
	}
	_, err = store.database.ExecContext(ctx, "INSERT INTO tunnels(id,host_id,document) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET host_id=excluded.host_id, document=excluded.document", spec.ID, spec.HostID, string(document))
	return err
}

func (store *Store) DeleteTunnel(ctx context.Context, identifier string) error {
	return store.deleteRow(ctx, "DELETE FROM tunnels WHERE id=?", identifier, fault.New(404, "tunnel_not_found", "隧道不存在。"))
}

func (store *Store) ListSnippets(ctx context.Context) ([]snippets.Snippet, error) {
	return listDocuments[snippets.Snippet](ctx, store, "SELECT document FROM snippets ORDER BY name COLLATE NOCASE, id")
}

func (store *Store) GetSnippet(ctx context.Context, identifier string) (snippets.Snippet, error) {
	return getDocument[snippets.Snippet](ctx, store, "SELECT document FROM snippets WHERE id=?", identifier, fault.New(404, "snippet_not_found", "命令片段不存在。"))
}

func (store *Store) PutSnippet(ctx context.Context, snippet snippets.Snippet) error {
	document, err := json.Marshal(snippet)
	if err != nil {
		return err
	}
	_, err = store.database.ExecContext(ctx, "INSERT INTO snippets(id,name,document) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET name=excluded.name, document=excluded.document", snippet.ID, snippet.Name, string(document))
	return err
}

func (store *Store) DeleteSnippet(ctx context.Context, identifier string) error {
	return store.deleteRow(ctx, "DELETE FROM snippets WHERE id=?", identifier, fault.New(404, "snippet_not_found", "命令片段不存在。"))
}
