// @title           Finance Tracker API
// @version         1.0
// @description     Personal finance tracking API for expenses, income, investments and balance snapshots.
// @host            localhost:8080
// @BasePath        /api/v1
// @schemes         http
package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/mindaugas/finance-tracker/internal/handler"
	"github.com/mindaugas/finance-tracker/internal/repository"
	"github.com/mindaugas/finance-tracker/internal/service"
	"github.com/mindaugas/finance-tracker/pkg/database"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	_ "github.com/mindaugas/finance-tracker/docs"
)

// Request-body caps: nothing in the regular API sends more than a few KB,
// while statement/backup uploads legitimately reach tens of MB.
const (
	maxBodyBytes       = 2 << 20  // 2 MiB
	maxImportBodyBytes = 64 << 20 // 64 MiB
)

func main() {
	dbPath := getEnv("DB_PATH", "finance.db")
	port := getEnv("PORT", "8080")

	db, err := database.NewSQLiteDB(dbPath)
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	// Repositories
	authRepo := repository.NewAuthRepository(db)
	budgetRepo := repository.NewBudgetRepository(db)
	txRepo := repository.NewTransactionRepository(db)
	balRepo := repository.NewBalanceRepository(db)
	insightRepo := repository.NewInsightRepository(db)
	stockRepo := repository.NewStockRepository(db)
	assetRepo := repository.NewAssetRepository(db)
	exportLogRepo := repository.NewExportLogRepository(db)

	// Services — balSvc must be created before txSvc (txSvc holds a reference to balSvc)
	authSvc := service.NewAuthService(authRepo)
	balSvc := service.NewBalanceService(balRepo, txRepo)
	txSvc := service.NewTransactionServiceWithRules(txRepo, balSvc, budgetRepo)
	stockSvc := service.NewStockService(stockRepo)
	assetSvc := service.NewAssetService(assetRepo)

	// Seed the AI gateway from the add-on options when the database carries
	// no key — a wiped or reinstalled instance comes back with AI working,
	// and a key the user set in the app is NEVER overwritten by the env.
	if key := os.Getenv("NEXOS_API_KEY"); key != "" {
		if s, err := insightRepo.GetAISettings(); err == nil && s.APIKey == "" {
			s.APIKey = key
			if m := os.Getenv("NEXOS_MODEL"); m != "" && s.Model == "" {
				s.Model = m
			}
			if u := os.Getenv("NEXOS_GATEWAY_URL"); u != "" {
				s.GatewayURL = u
			}
			if err := insightRepo.SaveAISettings(s); err == nil {
				log.Println("AI gateway seeded from add-on options")
			}
		}
	}
	// insightSvc reads budgets, stocks and assets for the AI report + chat tools.
	insightSvc := service.NewInsightService(insightRepo, txSvc, balSvc, budgetRepo, stockSvc, assetSvc)

	// Handlers
	authHandler := handler.NewAuthHandler(authSvc)
	budgetHandler := handler.NewBudgetHandler(budgetRepo)
	txHandler := handler.NewTransactionHandler(txSvc)
	balHandler := handler.NewBalanceHandler(balSvc)
	insightHandler := handler.NewInsightHandler(insightSvc)
	exportHandler := handler.NewExportHandler(txSvc, balSvc, stockSvc, assetSvc, exportLogRepo).WithBudgets(budgetRepo).WithAI(insightRepo)
	importHandler := handler.NewImportHandler(txRepo, balRepo, stockRepo, assetRepo).WithBudgets(budgetRepo).WithAI(insightRepo).WithDB(db)
	stockHandler := handler.NewStockHandler(stockSvc)
	assetHandler := handler.NewAssetHandler(assetSvc)

	r := gin.Default()

	// CORS — allow all origins for self-hosted deployment (nginx + local network access)
	r.Use(cors.New(cors.Config{
		AllowAllOrigins:  true,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept"},
	}))

	// Cap request bodies so a stray or malicious client can't exhaust
	// memory/disk; imports and the image-scan upload get a larger allowance
	// (the scan handler enforces its own tighter 10 MiB image cap on top).
	r.Use(func(c *gin.Context) {
		limit := int64(maxBodyBytes)
		p := c.Request.URL.Path
		if strings.HasPrefix(p, "/api/v1/import/") || p == "/api/v1/ai/scan-transaction" || p == "/api/v1/ai/chat" {
			limit = maxImportBodyBytes
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		c.Next()
	})

	// Swagger UI
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// API routes — auth routes register before the lock middleware so the
	// lock screen can talk to them; everything after is guarded.
	v1 := r.Group("/api/v1")
	authHandler.RegisterRoutes(v1)
	v1.Use(authHandler.Middleware())
	budgetHandler.RegisterRoutes(v1)
	txHandler.RegisterRoutes(v1)
	balHandler.RegisterRoutes(v1)
	insightHandler.RegisterRoutes(v1)
	exportHandler.RegisterRoutes(v1)
	importHandler.RegisterRoutes(v1)
	stockHandler.RegisterRoutes(v1)
	assetHandler.RegisterRoutes(v1)

	// Health check
	v1.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Nightly on-disk backups (VACUUM INTO) with retention — the SQLite file
	// is the only copy of the data, so it gets a standing safety net.
	database.StartBackupLoop(db, dbPath)

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: r,
		// WriteTimeout must cover the AI gateway proxy (backend caps the
		// gateway call at 120s; nginx allows 180s upstream read).
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		WriteTimeout:      180 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Printf("Server running on :%s — Swagger at http://localhost:%s/swagger/index.html", port, port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Graceful shutdown: finish in-flight writes (and let SQLite complete WAL
	// checkpoints) instead of being killed mid-request on add-on restarts.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down…")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("forced shutdown: %v", err)
	}
	log.Println("Server stopped")
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
