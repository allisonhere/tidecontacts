package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/allisonhere/tidecontacts/internal/app"
)

const mcpSchema = `
CREATE TABLE mcp_profiles(
 name TEXT PRIMARY KEY, enabled INTEGER NOT NULL CHECK(enabled IN (0,1)),
 scopes TEXT NOT NULL, revision INTEGER NOT NULL DEFAULT 1
);
CREATE TABLE mcp_audit(
 seq INTEGER PRIMARY KEY AUTOINCREMENT, at INTEGER NOT NULL, profile TEXT NOT NULL,
 tool TEXT NOT NULL, is_write INTEGER NOT NULL CHECK(is_write IN (0,1)), outcome TEXT NOT NULL
);
CREATE TRIGGER mcp_audit_limit AFTER INSERT ON mcp_audit WHEN NEW.seq % 128 = 0
BEGIN
 DELETE FROM mcp_audit WHERE seq <= NEW.seq - 10000;
END;
`

var _ app.MCPState = (*Store)(nil)

func scanMCPProfile(row interface{ Scan(...any) error }) (app.MCPProfile, error) {
	var p app.MCPProfile
	var scopes string
	if err := row.Scan(&p.Name, &p.Enabled, &scopes, &p.Revision); err != nil {
		return p, err
	}
	return p, json.Unmarshal([]byte(scopes), &p.Scopes)
}

func (s *Store) MCPProfile(ctx context.Context, name string) (app.MCPProfile, error) {
	return scanMCPProfile(s.db.QueryRowContext(ctx, `SELECT name,enabled,scopes,revision FROM mcp_profiles WHERE name=?`, name))
}

func (s *Store) MCPProfiles(ctx context.Context) ([]app.MCPProfile, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name,enabled,scopes,revision FROM mcp_profiles ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []app.MCPProfile{}
	for rows.Next() {
		p, err := scanMCPProfile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) SaveMCPProfile(ctx context.Context, p app.MCPProfile) (app.MCPProfile, error) {
	b, err := json.Marshal(p.Scopes)
	if err != nil {
		return p, err
	}
	err = s.write(ctx, func(tx *sql.Tx) error {
		var row *sql.Row
		const returning = ` RETURNING name,enabled,scopes,revision`
		if p.Revision == 0 {
			row = tx.QueryRowContext(ctx, `INSERT INTO mcp_profiles(name,enabled,scopes)
 SELECT ?,?,? WHERE (SELECT COUNT(*) FROM mcp_profiles)<64 ON CONFLICT(name) DO NOTHING`+returning, p.Name, p.Enabled, string(b))
		} else {
			row = tx.QueryRowContext(ctx, `UPDATE mcp_profiles SET enabled=?,scopes=?,revision=revision+1 WHERE name=? AND revision=?`+returning,
				p.Enabled, string(b), p.Name, p.Revision)
		}
		next, err := scanMCPProfile(row)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrConflict
		}
		if err != nil {
			return err
		}
		p = next
		_, err = tx.ExecContext(ctx, `INSERT INTO mcp_audit(at,profile,tool,is_write,outcome) VALUES(?,?,?,?,?)`,
			s.now().UnixMilli(), p.Name, "profile_update", true, "ok")
		return err
	})
	return p, err
}

func (s *Store) BeginMCPAudit(ctx context.Context, profile, tool string, write bool) (int64, error) {
	var seq int64
	err := s.db.QueryRowContext(ctx, `INSERT INTO mcp_audit(at,profile,tool,is_write,outcome) VALUES(?,?,?,?, 'started') RETURNING seq`,
		s.now().UnixMilli(), profile, tool, write).Scan(&seq)
	return seq, err
}

func (s *Store) FinishMCPAudit(ctx context.Context, sequence int64, outcome string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE mcp_audit SET outcome=? WHERE seq=? AND outcome='started'`, outcome, sequence)
	return err
}

func (s *Store) MCPAuditPage(ctx context.Context, before int64, limit int) ([]app.MCPAudit, error) {
	limit = max(1, min(limit, 100))
	rows, err := s.db.QueryContext(ctx, `SELECT seq,at,profile,tool,is_write,outcome FROM mcp_audit
 WHERE seq < COALESCE(NULLIF(?,0),9223372036854775807) ORDER BY seq DESC LIMIT ?`, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]app.MCPAudit, 0, limit)
	for rows.Next() {
		var a app.MCPAudit
		var stamp int64
		if err := rows.Scan(&a.Sequence, &stamp, &a.Profile, &a.Tool, &a.Write, &a.Outcome); err != nil {
			return nil, err
		}
		a.At = time.UnixMilli(stamp).UTC()
		out = append(out, a)
	}
	return out, rows.Err()
}
