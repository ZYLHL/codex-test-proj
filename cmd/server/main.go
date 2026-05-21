package main

import (
	"database/sql"
	"log"
	"net/http"
	"os"

	_ "modernc.org/sqlite"

	"sticky-notes/internal/api"
	"sticky-notes/internal/service"
	"sticky-notes/internal/store"
)

func main() {
	dbPath := getenv("SQLITE_PATH", "sticky-notes.db")
	token := getenv("API_TOKEN", "dev-token")
	addr := getenv("ADDR", ":8080")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	s := store.NewSQLiteStore(db)
	if err := s.Migrate(); err != nil {
		log.Fatal(err)
	}
	svc := service.NewSyncService(s)
	h := api.NewHandler(svc, s, token)

	log.Printf("listening on %s", addr)
	if err := http.ListenAndServe(addr, h.Routes()); err != nil {
		log.Fatal(err)
	}
}

func getenv(k, d string) string {
	v := os.Getenv(k)
	if v == "" {
		return d
	}
	return v
}
