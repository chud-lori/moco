package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/joho/godotenv"

	"moco/internal/server"
	"moco/internal/storage"
)

func main() {
	migrate := flag.Bool("migrate-storage", false, "copy local var/books/ files into the configured remote storage backend, then exit")
	flag.Parse()

	// Load .env file if present. Production envs usually pass real env vars.
	_ = godotenv.Load()

	addr := os.Getenv("MOCO_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	dataDir := os.Getenv("MOCO_DATA_DIR")
	if dataDir == "" {
		dataDir = "var"
	}

	backend, kind, err := buildStorageBackend(dataDir)
	if err != nil {
		log.Fatalf("storage backend: %v", err)
	}
	if prefix := strings.TrimSpace(os.Getenv("MOCO_STORAGE_PREFIX")); prefix != "" {
		backend = storage.WithPrefix(backend, prefix)
		kind += " [prefix=" + prefix + "]"
	}
	log.Printf("storage backend: %s", kind)

	srv := server.New(server.Config{
		Addr:               addr,
		DataDir:            dataDir,
		DBPath:             os.Getenv("MOCO_DB_PATH"),
		CookieName:         "moco_session",
		SecureCookies:      envBool("MOCO_SECURE_COOKIES"),
		PublicURL:          os.Getenv("MOCO_PUBLIC_URL"),
		Storage:            backend,
		StorageBaseDir:     dataDir,
		GoogleClientID:     os.Getenv("GOOGLE_OAUTH_CLIENT_ID"),
		GoogleClientSecret: os.Getenv("GOOGLE_OAUTH_CLIENT_SECRET"),
		GoogleRedirectURL:  os.Getenv("GOOGLE_OAUTH_REDIRECT_URL"),
	})

	if *migrate {
		log.Printf("running storage migration…")
		if err := srv.MigrateLocalToBackend(context.Background()); err != nil {
			log.Fatalf("migration failed: %v", err)
		}
		log.Printf("migration complete")
		return
	}

	log.Printf("moco listening on %s", addr)
	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}

// buildStorageBackend picks the storage backend based on MOCO_STORAGE.
// Default is "local" — so dev environments are safe to run with R2 creds
// sitting in .env without accidentally writing to the real bucket.
// Set MOCO_STORAGE=r2 in production to opt into the bucket.
func buildStorageBackend(dataDir string) (storage.Backend, string, error) {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("MOCO_STORAGE")))
	if mode == "" {
		mode = "local"
	}

	switch mode {
	case "local":
		return storage.NewLocal(dataDir), "local", nil
	case "r2":
		cfg := storage.R2Config{
			AccountID:       os.Getenv("MOCO_R2_ACCOUNT_ID"),
			AccessKeyID:     os.Getenv("MOCO_R2_ACCESS_KEY_ID"),
			SecretAccessKey: os.Getenv("MOCO_R2_SECRET_ACCESS_KEY"),
			Bucket:          os.Getenv("MOCO_R2_BUCKET"),
		}
		backend, err := storage.NewR2(context.Background(), cfg)
		if err != nil {
			return nil, "", err
		}
		return backend, "r2 (bucket=" + cfg.Bucket + ")", nil
	default:
		return nil, "", fmt.Errorf("unknown MOCO_STORAGE value %q (expected \"local\" or \"r2\")", mode)
	}
}

func envBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
