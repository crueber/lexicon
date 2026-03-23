//go:build dev

package server

import "io/fs"

// frontendFS is an empty filesystem used during development.
// In dev mode, the Vite dev server serves the frontend instead.
var frontendFS fs.FS = emptyFS{}

type emptyFS struct{}

func (emptyFS) Open(string) (fs.File, error) {
	return nil, fs.ErrNotExist
}
