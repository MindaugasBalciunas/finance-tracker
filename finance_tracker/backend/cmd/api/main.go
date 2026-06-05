// @title           Finance Tracker API
// @version         1.0
// @description     Personal finance tracking API for expenses, income, investments and balance snapshots.
// @host            localhost:8080
// @BasePath        /api/v1
// @schemes         http
package main

import (
	"log"
	"os"

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

func main() {
	dbPath := getEnv("DB_PATH", "finance.db")
	port := getEnv("PORT", "8080")

	db, err := database.NewSQLiteDB(dbPath)
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	// Repositories
	txRepo := repository.NewTransactionRepository(db)
	balRepo := repository.NewBalanceRepository(db)
	insightRepo := repository.NewInsightRepository(db)
	stockRepo := repository.NewStockRepository(db)
	exportLogRepo := repository.NewExportLogRepository(db)

	// Services
	txSvc := service.NewTransactionService(txRepo)
	balSvc := service.NewBalanceService(balRepo, txRepo)
	insightSvc := service.NewInsightService(insightRepo, txSvc, balSvc)
	stockSvc := service.NewStockService(stockRepo)

	// Handlers
	txHandler := handler.NewTransactionHandler(txSvc)
	balHandler := handler.NewBalanceHandler(balSvc)
	insightHandler := handler.NewInsightHandler(insightSvc)
	exportHandler := handler.NewExportHandler(txSvc, balSvc, stockSvc, exportLogRepo)
	importHandler := handler.NewImportHandler(txRepo, balRepo, stockRepo)
	stockHandler := handler.NewStockHandler(stockSvc)

	r := gin.Default()

	// CORS — allow all origins for self-hosted deployment (nginx + local network access)
	r.Use(cors.New(cors.Config{
		AllowAllOrigins:  true,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept"},
	}))

	// Swagger UI
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// API routes
	v1 := r.Group("/api/v1")
	txHandler.RegisterRoutes(v1)
	balHandler.RegisterRoutes(v1)
	insightHandler.RegisterRoutes(v1)
	exportHandler.RegisterRoutes(v1)
	importHandler.RegisterRoutes(v1)
	stockHandler.RegisterRoutes(v1)

	// Health check
	v1.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	log.Printf("Server running on :%s — Swagger at http://localhost:%s/swagger/index.html", port, port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
