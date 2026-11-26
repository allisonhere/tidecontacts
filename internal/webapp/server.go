// Package webapp serves the local graphical clients. It is not a network API.
package webapp

import (
	"bytes"
	"context"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/allisonhere/tidecontacts/internal/app"
	"github.com/allisonhere/tidecontacts/internal/domain"
	"github.com/allisonhere/tidecontacts/internal/ext"
	"github.com/allisonhere/tidecontacts/internal/storage/sqlite"
	"github.com/google/uuid"
)

// Store has the same data boundary as the terminal, plus revision-bound completion.
type Store interface {
	app.CRM
	CompleteTaskRevision(context.Context, string, int) (domain.Task, error)
	MoveItemRevision(context.Context, string, string, int) error
}

type Server struct {
	store Store
	demo  bool
	pro   *ext.Manager
}

type fault struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *fault) Error() string { return e.Message }

func invalid(message string) *fault { return &fault{400, "invalid_input", message} }
func notFound() *fault              { return &fault{404, "not_found", "This record or action does not exist."} }
func denied(message string) *fault  { return &fault{403, "permission_denied", message} }

func graphError(err error) *fault {
	var known *fault
	switch {
	case errors.As(err, &known):
		return known
	case errors.Is(err, sql.ErrNoRows):
		return notFound()
	case errors.Is(err, sqlite.ErrConflict):
		return &fault{409, "revision_conflict", "This record changed. Reload it before saving your draft again."}
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return &fault{503, "request_interrupted", "The request did not finish. Check the record before retrying a write."}
	default:
		return &fault{500, "internal_error", "TideContacts could not load or save this record."}
	}
}

type envelope struct {
	Version int             `json:"schema_version"`
	Data    any             `json:"data,omitempty"`
	Error   *fault          `json:"error,omitempty"`
	Meta    *projectionMeta `json:"meta,omitempty"`
}

func respond(w http.ResponseWriter, status int, data any, err error) {
	result := envelope{Version: 1, Data: data}
	if p, ok := data.(projection); ok {
		result.Data, result.Meta = p.Data, &p.Meta
	}
	if err != nil {
		result.Error = graphError(err)
		status, result.Data = result.Error.Status, nil
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(result)
}

// New binds requests to the exact listener address and a launch-specific bearer.
// assets may be nil for native clients. The caller keeps the rooted filesystem open.
func New(store Store, demo bool, host, token string, assets fs.FS, optional ...*ext.Manager) http.Handler {
	s := Server{store: store, demo: demo}
	if len(optional) > 0 {
		s.pro = optional[0]
	}
	auth := "Bearer " + token
	origin := "http://" + host
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'none'; connect-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; form-action 'none'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cache-Control", "no-store")
		if r.Host != host {
			respond(w, 0, nil, denied("Incorrect listener address."))
			return
		}
		if r.URL.Path != "/api/v1" && !strings.HasPrefix(r.URL.Path, "/api/v1/") {
			static(w, r, assets)
			return
		}
		if value := r.Header.Get("Origin"); value != "" && value != origin || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			respond(w, 0, nil, denied("This client origin is not allowed."))
			return
		}
		if token == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte(auth)) != 1 {
			respond(w, 0, nil, &fault{401, "session_required", "Open the current launch URL from TideContacts to connect."})
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		s.route(w, r.WithContext(ctx))
	})
}

func static(w http.ResponseWriter, r *http.Request, assets fs.FS) {
	if assets == nil || r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.NotFound(w, r)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/")
	parts := strings.Split(path, "/")
	if path == "" || len(parts) <= 2 && isWorkspace(parts[0]) && (len(parts) == 1 || validID(parts[1])) || isShellPage(path) {
		path = "index.html"
	} else if len(parts) != 2 || parts[0] != "assets" || !fs.ValidPath(path) || !(strings.HasSuffix(path, ".js") || strings.HasSuffix(path, ".css") || strings.HasSuffix(path, ".svg") || strings.HasSuffix(path, ".woff2")) {
		http.NotFound(w, r)
		return
	}
	b, err := fs.ReadFile(assets, path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if path != "index.html" {
		w.Header().Set("Cache-Control", "private, max-age=3600")
	}
	http.ServeContent(w, r, path, time.Time{}, bytes.NewReader(b))
}

func validID(id string) bool {
	u, err := uuid.Parse(id)
	return err == nil && u != uuid.Nil && u.String() == id
}

func isWorkspace(name string) bool {
	switch name {
	case "people", "organizations", "tasks", "activities", "projects", "pipelines":
		return true
	}
	return false
}

func isShellPage(path string) bool {
	switch path {
	case "settings", "sales", "business", "automation":
		return true
	}
	return false
}

func input[T any](w http.ResponseWriter, r *http.Request) (T, error) {
	var value T
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		return value, &fault{415, "json_required", "Send a JSON object with Content-Type: application/json."}
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(&value); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return value, &fault{413, "body_too_large", "The edit exceeds 64 KiB."}
		}
		return value, invalid("Invalid JSON edit: " + err.Error())
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return value, invalid("Send exactly one JSON object.")
	}
	return value, nil
}

func (s Server) route(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/"), "/")
	kind := parts[0]
	if kind == "profile" && len(parts) == 1 && r.Method == http.MethodGet {
		respond(w, 200, struct {
			Demo         bool     `json:"demo"`
			Professional []string `json:"professional"`
		}{s.demo, s.pro.GraphicalActions(r.Context())}, nil)
		return
	}
	var data any
	var err error
	status := http.StatusOK
	switch {
	case kind == "pro" && r.Method == http.MethodGet && (len(parts) == 2 || len(parts) == 3 && parts[2] != ""):
		id := ""
		if len(parts) == 3 {
			id = parts[2]
		}
		data, err = s.professional(r, parts[1], id)
	case len(parts) == 1 && isWorkspace(kind) && r.Method == http.MethodGet:
		data, err = s.list(r, kind)
	case len(parts) == 2 && isWorkspace(kind) && validID(parts[1]) && r.Method == http.MethodGet:
		data, err = s.record(r, kind, parts[1])
	case isWorkspace(kind) && kind != "pipelines" && (len(parts) == 1 && r.Method == http.MethodPost || len(parts) == 2 && validID(parts[1]) && r.Method == http.MethodPatch):
		id := ""
		if len(parts) == 2 {
			id = parts[1]
		} else {
			status = http.StatusCreated
		}
		data, err = s.save(w, r, kind, id)
	case r.Method == http.MethodPost && len(parts) == 3 && validID(parts[1]) && (kind == "tasks" && parts[2] == "complete" || kind == "items" && parts[2] == "move"):
		data, err = s.action(w, r, kind, parts[1])
	default:
		err = notFound()
	}
	respond(w, status, data, err)
}

func checkRevision(want, current int) error {
	if want < 1 {
		return invalid("The current record revision is required.")
	}
	if want != current {
		return sqlite.ErrConflict
	}
	return nil
}

func textError(field string, maxLen int) error {
	return invalid(fmt.Sprintf("%s is required and must be at most %d bytes.", field, maxLen))
}
