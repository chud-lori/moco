package main

import (
	"log"
	"net/http"
	"os"

	"moco/internal/server"
)

func main() {
	addr := os.Getenv("MOCO_ADDR")
	if addr == "" {
		addr = ":8080"
	}

	srv := server.New(server.Config{
		Addr:       addr,
		DataDir:    os.Getenv("MOCO_DATA_DIR"),
		DBPath:     os.Getenv("MOCO_DB_PATH"),
		CookieName: "moco_session",
	})

	log.Printf("moco listening on %s", addr)

	if err := http.ListenAndServe(addr, srv.Handler()); err != nil {
		log.Fatal(err)
	}
}
