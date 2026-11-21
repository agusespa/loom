package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
	_ "time/tzdata" // the distroless image has no zoneinfo

	"github.com/agusespa/loom/internal/api"
	"github.com/agusespa/loom/internal/store"
	"github.com/agusespa/loom/internal/ui"
)

func main() {
	if len(os.Args) > 1 {
		if len(os.Args) != 2 || os.Args[1] != "healthcheck" {
			log.Print("usage: loom [healthcheck]")
			os.Exit(2)
		}
		if err := checkReadiness(context.Background(), "http://127.0.0.1:"+env("PORT", "8080")+"/api/readyz"); err != nil {
			log.Printf("healthcheck: %v", err)
			os.Exit(1)
		}
		return
	}

	dbPath := env("DB_PATH", "data/loom.db")
	port := env("PORT", "8080")

	loc, err := time.LoadLocation(env("TIMEZONE", "Europe/Stockholm"))
	if err != nil {
		log.Fatalf("TIMEZONE: %v", err)
	}
	dayStart, err := strconv.Atoi(env("DAY_START_HOUR", "5"))
	if err != nil || dayStart < 0 || dayStart > 23 {
		log.Fatal("DAY_START_HOUR must be 0-23")
	}

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		log.Fatalf("create data dir: %v", err)
	}
	s, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	defer func() { _ = s.Close() }()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	clock := api.Clock{Loc: loc, DayStart: time.Duration(dayStart) * time.Hour}
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           api.New(s, ui.FS(), clock),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()

	log.Printf("loom listening on :%s (db %s, %s, day starts %02d:00)", port, dbPath, loc, dayStart)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("serve: %v", err)
	}
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
