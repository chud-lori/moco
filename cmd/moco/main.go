package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"moco/internal/server"
)

func main() {
	addr := os.Getenv("MOCO_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	srv := server.New(server.Config{
		Addr:          addr,
		DataDir:       os.Getenv("MOCO_DATA_DIR"),
		DBPath:        os.Getenv("MOCO_DB_PATH"),
		CookieName:    "moco_session",
		SecureCookies: envBool("MOCO_SECURE_COOKIES"),
	})

	log.Printf("moco listening on %s", addr)

	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		log.Fatal(err)
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
