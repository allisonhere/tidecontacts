package ext

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/allisonhere/tidecontacts/internal/app"
)

// eventJournal keeps a consumer on its declared topics and current read grants.
// Consumer identity comes from the approved manifest, never from the request.
func (s *Session) eventJournal(ctx context.Context, method string, data json.RawMessage) (any, *ProtocolError) {
	if err := s.need("events.consume"); err != nil {
		return nil, err
	}
	journal, ok := s.Repo.(app.EventJournal)
	if !ok {
		return nil, perr(CodeUnknownMethod, "durable events are not available")
	}
	types := make([]string, 0, len(s.durableTypes))
	for _, typ := range s.durableTypes {
		if info, ok := LookupEventType(typ); ok && s.Granted.Has(info.ReadPermission) {
			types = append(types, typ)
		}
	}
	if len(types) == 0 {
		return nil, perr(CodePermissionDenied, "no declared event topics have their matching read permissions")
	}
	switch method {
	case "events.next":
		if err := decode(data, &struct{}{}); err != nil {
			return nil, err
		}
		delivery, err := journal.NextEvent(ctx, s.ID, types)
		if err != nil {
			return nil, mapErr(err)
		}
		return delivery, nil
	case "events.dead":
		var a struct {
			After int64 `json:"after"`
			Limit int   `json:"limit"`
		}
		if err := decode(data, &a); err != nil {
			return nil, err
		}
		if a.After < 0 || a.Limit < 0 || a.Limit > 100 {
			return nil, perr(CodeInvalid, "after must be nonnegative and limit at most 100")
		}
		items, err := journal.DeadEvents(ctx, s.ID, types, a.After, limitOf(a.Limit, 50, 100))
		if err != nil {
			return nil, mapErr(err)
		}
		return items, nil
	case "events.settle", "events.replay":
		var a struct {
			Sequence int64 `json:"sequence"`
			Retry    bool  `json:"retry"`
		}
		if err := decode(data, &a); err != nil {
			return nil, err
		}
		if a.Sequence <= 0 || method == "events.replay" && a.Retry {
			return nil, perr(CodeInvalid, "invalid delivery sequence or outcome")
		}
		var err error
		if method == "events.replay" {
			err = journal.ReplayEvent(ctx, s.ID, types, a.Sequence)
		} else {
			err = journal.SettleEvent(ctx, s.ID, types, a.Sequence, a.Retry)
		}
		if err != nil {
			if errors.Is(err, app.ErrEventNotEligible) {
				return nil, perr(CodeConflict, "event is not eligible for this delivery operation")
			}
			return nil, mapErr(err)
		}
		return map[string]bool{"ok": true}, nil
	}
	return nil, perr(CodeUnknownMethod, "unknown event journal operation")
}
