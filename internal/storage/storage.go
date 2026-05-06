// Package storage abstracts persistent file storage so the rest of the app
// doesn't care whether files live on the local filesystem or in an
// S3-compatible object store like Cloudflare R2.
//
// The "key" is a logical path (e.g., "books/u_abc/b_xyz/original.pdf").
// Local backend translates keys to absolute filesystem paths under DataDir;
// R2 backend uses them as object keys.
package storage

import (
	"context"
	"io"
)

// LocalPathOf returns the absolute filesystem path that backend uses for the
// given key, if (and only if) backend resolves to a Local store — possibly
// wrapped in Prefixed. Returns "" for remote backends. Used by streaming
// handlers to take advantage of http.ServeFile's range-request support.
func LocalPathOf(b Backend, key string) string {
	switch v := b.(type) {
	case *Local:
		return v.absPath(key)
	case *Prefixed:
		return LocalPathOf(v.inner, v.prefix+key)
	}
	return ""
}

// Backend is the small surface area we need from object storage.
type Backend interface {
	// Put writes body to key. Existing objects are overwritten.
	Put(ctx context.Context, key string, body io.Reader, contentType string, size int64) error

	// Get returns a reader for the object. Caller must Close.
	Get(ctx context.Context, key string) (io.ReadCloser, error)

	// Stat returns size + existence of an object. (size=0, exists=false, nil) for missing.
	Stat(ctx context.Context, key string) (size int64, exists bool, err error)

	// Delete removes a single object. Missing objects don't error.
	Delete(ctx context.Context, key string) error

	// DeletePrefix removes every object under prefix. Used when removing a book.
	DeletePrefix(ctx context.Context, prefix string) error

	// LocalPath returns a filesystem path for tools that can only operate on
	// disk (Calibre, mutool, http.ServeFile fast path). For local backends
	// this is the real path with cleanup=nil. For remote backends the object
	// is downloaded to a temp file; cleanup must be invoked when done.
	LocalPath(ctx context.Context, key string) (path string, cleanup func(), err error)
}
