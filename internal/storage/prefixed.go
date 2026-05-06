package storage

import (
	"context"
	"io"
	"strings"
)

// Prefixed wraps a Backend so every operation transparently prepends a fixed
// prefix to the key. Used to share one bucket across environments
// (e.g. "dev/" vs "prod/") without code-level branching.
type Prefixed struct {
	inner  Backend
	prefix string
}

// WithPrefix returns a Backend that prepends prefix to every key. An empty
// prefix passes the backend through unchanged. A non-empty prefix is
// normalized to end in a single trailing slash.
func WithPrefix(b Backend, prefix string) Backend {
	prefix = strings.Trim(prefix, "/ \t\r\n")
	if prefix == "" {
		return b
	}
	return &Prefixed{inner: b, prefix: prefix + "/"}
}

func (p *Prefixed) k(key string) string {
	return p.prefix + key
}

func (p *Prefixed) Put(ctx context.Context, key string, body io.Reader, contentType string, size int64) error {
	return p.inner.Put(ctx, p.k(key), body, contentType, size)
}

func (p *Prefixed) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	return p.inner.Get(ctx, p.k(key))
}

func (p *Prefixed) Stat(ctx context.Context, key string) (int64, bool, error) {
	return p.inner.Stat(ctx, p.k(key))
}

func (p *Prefixed) Delete(ctx context.Context, key string) error {
	return p.inner.Delete(ctx, p.k(key))
}

func (p *Prefixed) DeletePrefix(ctx context.Context, prefix string) error {
	return p.inner.DeletePrefix(ctx, p.k(prefix))
}

func (p *Prefixed) LocalPath(ctx context.Context, key string) (string, func(), error) {
	return p.inner.LocalPath(ctx, p.k(key))
}
