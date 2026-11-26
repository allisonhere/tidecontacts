package webapp

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/allisonhere/tidecontacts/internal/ext"
)

type projectionMeta struct {
	Count             int  `json:"count"`
	PossiblyTruncated bool `json:"possibly_truncated"`
}

// projection carries an extension answer together with its paging metadata.
type projection struct {
	Data any
	Meta projectionMeta
}

// professional serves one authorized, read-only Professional operation. The
// route is a closed set (see ext.GraphicalActions): nothing from the request
// becomes a command, a path or a mutation.
func (s Server) professional(r *http.Request, operation, id string) (any, error) {
	q := r.URL.Query()
	query := ext.GraphicalQuery{
		Operation: operation,
		ID:        id,
		Period:    q.Get("period"),
		Currency:  q.Get("currency"),
		Query:     q.Get("q"),
		Date:      q.Get("date"),
	}
	if raw := q.Get("offset"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			return nil, invalid("The offset must be a non-negative number.")
		}
		query.Offset = n
	}
	res, err := s.pro.RunGraphicalRead(r.Context(), query)
	if err != nil {
		var pe *ext.ProtocolError
		if errors.As(err, &pe) {
			switch pe.Code {
			case ext.CodePermissionDenied, ext.CodeEntitlement:
				return nil, denied("This Professional view is not available.")
			case ext.CodeConflict:
				return nil, &fault{409, "revision_conflict", "This record changed. Reload it and try again."}
			case ext.CodeNotFound:
				return nil, notFound()
			case ext.CodeInvalid:
				return nil, invalid(pe.Message)
			}
		}
		return nil, err
	}
	return projection{Data: res.Data, Meta: projectionMeta{Count: res.Count, PossiblyTruncated: res.Truncated}}, nil
}
