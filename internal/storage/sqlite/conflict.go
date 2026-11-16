package sqlite

import "errors"

// ErrConflict identifies an optimistic concurrency failure across UI, plugin and CLI.
var ErrConflict = errors.New("record changed")
