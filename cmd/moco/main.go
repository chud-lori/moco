package main

import (
	"context"
	"flag"
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
		Addr:           addr,
		DataDir:        dataDir,
		DBPath:         os.Getenv("MOCO_DB_PATH"),
		CookieName:     "moco_session",
		SecureCookies:  envBool("MOCO_SECURE_COOKIES"),
		PublicURL:      os.Getenv("MOCO_PUBLIC_URL"),
		Storage:        backend,
		StorageBaseDir: dataDir,
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

// buildStorageBackend picks local-fs or R2 based on env vars. R2 is selected
// when all four MOCO_R2_* variables are present. Otherwise fallback to local.
func buildStorageBackend(dataDir string) (storage.Backend, string, error) {
	r2Account := os.Getenv("MOCO_R2_ACCOUNT_ID")
	r2Access := os.Getenv("MOCO_R2_ACCESS_KEY_ID")
	r2Secret := os.Getenv("MOCO_R2_SECRET_ACCESS_KEY")
	r2Bucket := os.Getenv("MOCO_R2_BUCKET")

	if strings.EqualFold(os.Getenv("MOCO_STORAGE"), "local") {
		return storage.NewLocal(dataDir), "local (forced)", nil
	}
	if r2Account != "" && r2Access != "" && r2Secret != "" && r2Bucket != "" {
		backend, err := storage.NewR2(context.Background(), storage.R2Config{
			AccountID:       r2Account,
			AccessKeyID:     r2Access,
			SecretAccessKey: r2Secret,
			Bucket:          r2Bucket,
		})
		if err != nil {
			return nil, "", err
		}
		return backend, "r2 (bucket=" + r2Bucket + ")", nil
	}
	return storage.NewLocal(dataDir), "local", nil
}

func envBool(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
