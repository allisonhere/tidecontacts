package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"time"

	"github.com/allisonhere/tidecontacts/internal/app"
	"github.com/google/uuid"
)

const eventLogSchema = `
CREATE TABLE event_log (
 sequence INTEGER PRIMARY KEY AUTOINCREMENT,
 id TEXT NOT NULL UNIQUE,
 type TEXT NOT NULL,
 schema_version INTEGER NOT NULL,
 source TEXT NOT NULL,
 entity_type TEXT NOT NULL,
 entity_id TEXT NOT NULL,
 occurred_at INTEGER NOT NULL,
 recorded_at INTEGER NOT NULL,
 revision INTEGER NOT NULL,
 related_id TEXT NOT NULL,
 correlation_id TEXT NOT NULL,
 causation_id TEXT NOT NULL,
 automation_id TEXT NOT NULL,
 depth INTEGER NOT NULL,
 payload TEXT NOT NULL
);
CREATE INDEX event_log_type_sequence ON event_log(type,sequence);
CREATE INDEX event_log_recorded_at ON event_log(recorded_at);
CREATE TABLE event_checkpoints (
 consumer TEXT NOT NULL,
 type TEXT NOT NULL,
 sequence INTEGER NOT NULL,
 PRIMARY KEY(consumer,type)
) WITHOUT ROWID;
CREATE INDEX event_checkpoints_type_sequence ON event_checkpoints(type,sequence);
CREATE TABLE event_delivery_state (
 consumer TEXT NOT NULL,
 sequence INTEGER NOT NULL REFERENCES event_log(sequence),
 state TEXT NOT NULL CHECK(state IN ('pending','dead','replay')),
 attempts INTEGER NOT NULL DEFAULT 0,
 ready_at INTEGER NOT NULL DEFAULT 0,
 offered INTEGER NOT NULL DEFAULT 0,
 PRIMARY KEY(consumer,sequence)
) WITHOUT ROWID;
CREATE INDEX event_delivery_state_by_state ON event_delivery_state(consumer,state,sequence);
CREATE INDEX event_delivery_state_by_sequence ON event_delivery_state(sequence);
`

var eventToken = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,63}$`)

// appendEvent takes the business transaction: a failed write or a failed append
// must roll back both. All times describe the mutation, not the content of the
// record (an activity, for instance, can describe a much earlier interaction).
func appendEvent(ctx context.Context, tx *sql.Tx, now time.Time, event app.DurableEvent) error {
	if app.EventsSuppressed(ctx) {
		return nil
	}
	origin := app.EventOriginFromContext(ctx)
	if origin.Source != "" {
		event.Source = origin.Source
	} else if event.Source == "" {
		event.Source = "local"
	}
	for _, id := range [...]string{event.Source, event.EntityID} {
		if !eventToken.MatchString(id) {
			return fmt.Errorf("invalid event source or entity identifier")
		}
	}
	for _, id := range [...]string{event.RelatedID, origin.CorrelationID, origin.CausationID, origin.AutomationID} {
		if id != "" && !eventToken.MatchString(id) {
			return fmt.Errorf("invalid related event or lineage identifier")
		}
	}
	if origin.Depth < 0 || origin.Depth > 32 {
		return fmt.Errorf("event depth must be between 0 and 32")
	}
	payload := ""
	if event.Payload != nil {
		b, err := json.Marshal(event.Payload)
		if err != nil {
			return err
		}
		payload = string(b)
	}
	at := now.UnixMilli()
	_, err := tx.ExecContext(ctx, `INSERT INTO event_log(id,type,schema_version,source,entity_type,entity_id,occurred_at,recorded_at,revision,related_id,correlation_id,causation_id,automation_id,depth,payload)
 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, uuid.NewString(), event.Type, app.DurableEventSchemaVersion, event.Source, event.EntityType, event.EntityID, at, at,
		event.Revision, event.RelatedID, origin.CorrelationID, origin.CausationID, origin.AutomationID, origin.Depth, payload)
	return err
}

const deliveryColumns = `e.sequence,e.id,e.type,e.schema_version,e.source,e.entity_type,e.entity_id,e.occurred_at,e.recorded_at,
 e.revision,e.related_id,e.correlation_id,e.causation_id,e.automation_id,e.depth,e.payload`

func readDelivery(row interface{ Scan(...any) error }) (app.EventDelivery, error) {
	var d app.EventDelivery
	var occurred, recorded int64
	var payload string
	e := &d.Event
	err := row.Scan(&d.Sequence, &e.ID, &e.Type, &e.SchemaVersion, &e.Source, &e.EntityType, &e.EntityID, &occurred, &recorded,
		&e.Revision, &e.RelatedID, &e.CorrelationID, &e.CausationID, &e.AutomationID, &e.Depth, &payload, &d.Attempts)
	if err != nil {
		return d, err
	}
	e.OccurredAt = time.UnixMilli(occurred).UTC().Format(time.RFC3339Nano)
	e.RecordedAt = time.UnixMilli(recorded).UTC().Format(time.RFC3339Nano)
	if payload != "" {
		err = json.Unmarshal([]byte(payload), &e.Payload)
	}
	return d, err
}

func (s *Store) NextEvent(ctx context.Context, consumer string, types []string) (*app.EventDelivery, error) {
	if len(types) == 0 {
		return nil, nil
	}
	topics, err := json.Marshal(types)
	if err != nil {
		return nil, err
	}
	var result *app.EventDelivery
	err = s.write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `INSERT INTO event_checkpoints(consumer,type,sequence)
 SELECT ?,value,0 FROM json_each(?) WHERE true ON CONFLICT(consumer,type) DO NOTHING`, consumer, string(topics)); err != nil {
			return err
		}
		now := s.now().UnixMilli()
		d, err := readDelivery(tx.QueryRowContext(ctx, `
WITH candidates(sequence) AS (
 SELECT (SELECT sequence FROM event_log WHERE type=t.value AND sequence>COALESCE(c.sequence,0) ORDER BY sequence LIMIT 1)
 FROM json_each(?) t LEFT JOIN event_checkpoints c ON c.type=t.value AND c.consumer=?
 UNION ALL
 SELECT d.sequence FROM event_delivery_state d JOIN event_log e ON e.sequence=d.sequence
 WHERE d.consumer=? AND d.state='replay' AND e.type IN (SELECT value FROM json_each(?))
)
SELECT `+deliveryColumns+`,COALESCE(d.attempts,0)
FROM candidates c JOIN event_log e ON e.sequence=c.sequence
LEFT JOIN event_delivery_state d ON d.sequence=c.sequence AND d.consumer=?
WHERE COALESCE(d.ready_at,0)<=? AND COALESCE(d.state,'pending')!='dead'
ORDER BY e.sequence LIMIT 1`, string(topics), consumer, consumer, string(topics), consumer, now))
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO event_delivery_state(consumer,sequence,state,offered)
 VALUES(?,?,'pending',1) ON CONFLICT(consumer,sequence) DO UPDATE SET offered=1`, consumer, d.Sequence)
		if err == nil {
			result = &d
		}
		return err
	})
	return result, err
}

func deliveryTopic(ctx context.Context, tx *sql.Tx, types []string, seq int64) (string, error) {
	var topic string
	err := tx.QueryRowContext(ctx, `SELECT type FROM event_log WHERE sequence=?`, seq).Scan(&topic)
	if errors.Is(err, sql.ErrNoRows) || err == nil && !slices.Contains(types, topic) {
		return "", app.ErrEventNotEligible
	}
	return topic, err
}

const eventMaxAttempts = 5

func (s *Store) SettleEvent(ctx context.Context, consumer string, types []string, sequence int64, retry bool) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		// Acquire SQLite's writer before reading, including across processes.
		if _, err := tx.ExecContext(ctx, `UPDATE event_checkpoints SET sequence=sequence WHERE 0`); err != nil {
			return err
		}
		topic, err := deliveryTopic(ctx, tx, types, sequence)
		if err != nil {
			return err
		}
		var state string
		var attempts int
		var offered bool
		err = tx.QueryRowContext(ctx, `SELECT state,attempts,offered FROM event_delivery_state WHERE consumer=? AND sequence=?`, consumer, sequence).
			Scan(&state, &attempts, &offered)
		if errors.Is(err, sql.ErrNoRows) && !retry {
			var ack int64
			err = tx.QueryRowContext(ctx, `SELECT sequence FROM event_checkpoints WHERE consumer=? AND type=?`, consumer, topic).Scan(&ack)
			if err == nil && ack >= sequence {
				return nil
			}
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			return app.ErrEventNotEligible
		}
		if errors.Is(err, sql.ErrNoRows) || err == nil && (!offered || state == "dead") {
			return app.ErrEventNotEligible
		}
		if err != nil {
			return err
		}
		if retry {
			attempts++
			if attempts < eventMaxAttempts {
				ready := s.now().Add(time.Second << (attempts - 1)).UnixMilli()
				_, err = tx.ExecContext(ctx, `UPDATE event_delivery_state SET attempts=?,ready_at=?,offered=0 WHERE consumer=? AND sequence=?`, attempts, ready, consumer, sequence)
				return err
			}
			if _, err = tx.ExecContext(ctx, `UPDATE event_delivery_state SET state='dead',attempts=?,offered=0 WHERE consumer=? AND sequence=?`, attempts, consumer, sequence); err != nil {
				return err
			}
		} else if _, err = tx.ExecContext(ctx, `DELETE FROM event_delivery_state WHERE consumer=? AND sequence=?`, consumer, sequence); err != nil {
			return err
		}
		if state == "replay" {
			return nil // replay never rewinds or moves a topic checkpoint
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO event_checkpoints(consumer,type,sequence) VALUES(?,?,?)
 ON CONFLICT(consumer,type) DO UPDATE SET sequence=MAX(sequence,excluded.sequence)`, consumer, topic, sequence)
		return err
	})
}

func (s *Store) DeadEvents(ctx context.Context, consumer string, types []string, after int64, limit int) ([]app.EventDelivery, error) {
	topics, err := json.Marshal(types)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT `+deliveryColumns+`,d.attempts FROM event_delivery_state d
 JOIN event_log e ON e.sequence=d.sequence WHERE d.consumer=? AND d.state='dead' AND d.sequence>?
 AND e.type IN (SELECT value FROM json_each(?)) ORDER BY d.sequence LIMIT ?`, consumer, after, string(topics), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []app.EventDelivery{}
	for rows.Next() {
		d, err := readDelivery(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) ReplayEvent(ctx context.Context, consumer string, types []string, sequence int64) error {
	return s.write(ctx, func(tx *sql.Tx) error {
		if _, err := deliveryTopic(ctx, tx, types, sequence); err != nil {
			return err
		}
		result, err := tx.ExecContext(ctx, `UPDATE event_delivery_state SET state='replay',attempts=0,ready_at=0,offered=0
 WHERE consumer=? AND sequence=? AND state='dead'`, consumer, sequence)
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err == nil && n != 1 {
			return app.ErrEventNotEligible
		}
		return err
	})
}

// PruneEvents removes up to 1,000 acknowledged envelopes older than 180 days.
// A topic must have a checkpoint and must be past every enrolled consumer's
// cursor. Pending and dead-letter envelopes always pin their rows.
func (s *Store) PruneEvents(ctx context.Context) (int64, error) {
	cutoff := s.now().AddDate(0, 0, -180).UnixMilli()
	result, err := s.db.ExecContext(ctx, `DELETE FROM event_log WHERE sequence IN (
 SELECT e.sequence FROM event_log e WHERE e.recorded_at<? AND
 e.sequence <= (SELECT MIN(c.sequence) FROM event_checkpoints c WHERE c.type=e.type) AND
 NOT EXISTS (SELECT 1 FROM event_delivery_state d WHERE d.sequence=e.sequence)
 ORDER BY e.recorded_at LIMIT 1000)`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
