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
	"ft/internal/db"
	"ft/internal/importv1"
	"ft/internal/ledger"
)

var version = "dev"

func main() {
	path := envOr("DB_PATH", "finance-v2.db")
	legacy := envOr("V1_DB_PATH", filepath.Join(filepath.Dir(path), "finance.db"))
	_, statErr := os.Stat(path)
	fresh := os.IsNotExist(statErr)

	d, err := db.Open(path)
	if err != nil {
		log.Fatalf("opening database: %v", err)
	}
	defer d.Close()

	if fresh {
		// First start of v2. If a v1 database sits next to us, convert it;
		// the v1 file is only read, so reinstalling v1 rolls back cleanly.
		if _, err := os.Stat(legacy); err == nil && legacy != path {
			log.Printf("first start: converting v1 database %s", legacy)
			src, err := importv1.ReadDB(legacy)
			if err != nil {
				os.Remove(path)
				log.Fatalf("reading v1 database: %v", err)
			}
			rep, err := importv1.Convert(d, src)
			if err != nil {
				d.Close()
				os.Remove(path)
				log.Fatalf("converting v1 database: %v", err)
			}
			log.Printf("converted: %d transactions, %d balance points, %d accounts, %d rules, %d budgets, %d trades, %d inbox rows",
				rep.Transactions, rep.BalanceRows, rep.Accounts, rep.Rules, rep.Budgets, rep.Trades, rep.InboxRows)
			for _, w := range rep.Warnings {
				log.Printf("  note: %s", w)
			}
		} else if err := ledger.SeedCategories(d); err != nil {
			log.Fatalf("seeding categories: %v", err)
		}
	}

	go nightly(d, path)

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
