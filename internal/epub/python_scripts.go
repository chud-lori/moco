package epub

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

//go:embed scripts/*.py
var pythonScriptFS embed.FS

func writeEmbeddedPythonScript(prefix, name string) (string, func(), error) {
	body, err := pythonScriptFS.ReadFile("scripts/" + name)
	if err != nil {
		return "", func() {}, fmt.Errorf("read embedded python script %s: %w", name, err)
	}

	tmpDir, err := os.MkdirTemp("", prefix)
	if err != nil {
		return "", func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }

	scriptPath := filepath.Join(tmpDir, name)
	if err := os.WriteFile(scriptPath, body, 0o600); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return scriptPath, cleanup, nil
}
