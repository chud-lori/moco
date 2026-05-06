package storage

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
)

// Local stores files under a base directory on the host filesystem.
type Local struct {
	BaseDir string
}

func NewLocal(baseDir string) *Local {
	return &Local{BaseDir: baseDir}
}

func (l *Local) absPath(key string) string {
	return filepath.Join(l.BaseDir, filepath.FromSlash(key))
}

func (l *Local) Put(_ context.Context, key string, body io.Reader, _ string, _ int64) error {
	full := l.absPath(key)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	f, err := os.Create(full)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, body)
	return err
}

func (l *Local) Get(_ context.Context, key string) (io.ReadCloser, error) {
	return os.Open(l.absPath(key))
}

func (l *Local) Stat(_ context.Context, key string) (int64, bool, error) {
	info, err := os.Stat(l.absPath(key))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, false, nil
		}
		return 0, false, err
	}
	return info.Size(), true, nil
}

func (l *Local) Delete(_ context.Context, key string) error {
	err := os.Remove(l.absPath(key))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (l *Local) DeletePrefix(_ context.Context, prefix string) error {
	target := l.absPath(prefix)
	err := os.RemoveAll(target)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (l *Local) LocalPath(_ context.Context, key string) (string, func(), error) {
	return l.absPath(key), nil, nil
}
