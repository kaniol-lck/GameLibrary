package library

import "errors"

// ErrNotFound is returned when an operation targets a game ID that is not in
// the cache.
var ErrNotFound = errors.New("game not found")
