// Finance Tracker server: JSON API on :8080 behind nginx.
package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"ft/internal/api"
	"ft/internal/boot"
	"ft/internal/db"
	"ft/internal/ibkr"
)

var version = "dev"

func main() {
	path := envOr("DB_PATH", "finance-v2.db")
	legacy := envOr("V1_DB_PATH", filepath.Join(filepath.Dir(path), "finance.db"))
	d, _, err := boot.Open(path, legacy)
	if err != nil {
		log.Fatalf("starting: %v", err)
	}
	defer d.Close()

	go nightly(d, path)
	go ibkrDaily(d)

	srv := api.New(d, path, version)
	addr := ":" + envOr("PORT", "8080")
	hs := &http.Server{Addr: addr, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second, WriteTimeout: 300 * time.Second, IdleTimeout: 120 * time.Second}
	go func() {
		log.Printf("finance tracker %s listening on %s (db %s)", version, addr, path)
		if err := hs.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	hs.Shutdown(ctx)
}

// nightly keeps a daily VACUUM INTO snapshot (14 kept).
func nightly(d *sql.DB, path string) {
	for {
		if time.Since(db.LatestBackup(path, "nightly")) > 23*time.Hour {
			if err := db.Backup(d, path, "nightly"); err != nil {
				log.Printf("nightly backup failed: %v", err)
			}
		}
		time.Sleep(time.Hour)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// ibkrDaily refreshes the IBKR account value once a day when it is
// connected (only today's balance; trades are proposed on a manual sync).
// Using the sign-in daily also keeps it from lapsing.
func ibkrDaily(d *sql.DB) {
	svc := ibkr.New(d)
	time.Sleep(2 * time.Minute) // let start-up settle
	for {
		if st := svc.Status(); st.Connected {
			last, _ := time.Parse(time.RFC3339, st.LastSync)
			if time.Since(last) > 20*time.Hour {
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				if _, err := svc.Sync(ctx, true); err != nil {
					log.Printf("ibkr daily refresh: %v", err)
				}
				cancel()
			}
		}
		time.Sleep(time.Hour)
	}
}
