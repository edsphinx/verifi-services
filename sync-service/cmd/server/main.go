package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/verifi-protocol/sync-service/internal/config"
	"github.com/verifi-protocol/sync-service/internal/db"
	"github.com/verifi-protocol/sync-service/internal/indexer"
	"github.com/verifi-protocol/sync-service/internal/sync"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Warn().Msg("No .env file found, using system environment variables")
	}

	// Setup logger
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	log.Info().Msg("🚀 VeriFi Sync Service Starting...")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	// Initialize database
	database, err := db.New(cfg.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database")
	}
	defer database.Close()

	log.Info().Msg("✅ Database connected")

	// Initialize sync service
	syncService := sync.NewService(database, cfg)

	// Setup Fiber app
	app := fiber.New(fiber.Config{
		AppName:      "VeriFi Sync Service",
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	})

	// Middleware
	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST",
	}))

	// Health check
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status":  "healthy",
			"service": "verifi-sync-service",
			"time":    time.Now().Unix(),
		})
	})

	// Manual sync endpoints
	app.Post("/sync/metrics", func(c *fiber.Ctx) error {
		log.Info().Msg("📊 Manual metrics sync triggered")
		if err := syncService.SyncMetrics(context.Background()); err != nil {
			log.Error().Err(err).Msg("Metrics sync failed")
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"status": "success", "message": "Metrics synced"})
	})

	app.Post("/sync/pools", func(c *fiber.Ctx) error {
		log.Info().Msg("💧 Manual pools sync triggered")
		if err := syncService.SyncPools(context.Background()); err != nil {
			log.Error().Err(err).Msg("Pools sync failed")
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"status": "success", "message": "Pools synced"})
	})

	app.Post("/sync/activities", func(c *fiber.Ctx) error {
		log.Info().Msg("📝 Manual activities sync triggered")
		if err := syncService.SyncActivities(context.Background()); err != nil {
			log.Error().Err(err).Msg("Activities sync failed")
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(fiber.Map{"status": "success", "message": "Activities synced"})
	})

	// Status endpoint
	app.Get("/status", func(c *fiber.Ctx) error {
		stats := syncService.GetStats()
		return c.JSON(stats)
	})

	// Setup cron jobs
	cronScheduler := cron.New(cron.WithSeconds())

	// Metrics sync - every hour
	cronScheduler.AddFunc("0 0 * * * *", func() {
		log.Info().Msg("⏰ Running scheduled metrics sync")
		if err := syncService.SyncMetrics(context.Background()); err != nil {
			log.Error().Err(err).Msg("Scheduled metrics sync failed")
		}
	})

	// Pools sync - every 15 minutes
	cronScheduler.AddFunc("0 */15 * * * *", func() {
		log.Info().Msg("⏰ Running scheduled pools sync")
		if err := syncService.SyncPools(context.Background()); err != nil {
			log.Error().Err(err).Msg("Scheduled pools sync failed")
		}
	})

	// Activities sync - every 5 minutes
	cronScheduler.AddFunc("0 */5 * * * *", func() {
		log.Info().Msg("⏰ Running scheduled activities sync")
		if err := syncService.SyncActivities(context.Background()); err != nil {
			log.Error().Err(err).Msg("Scheduled activities sync failed")
		}
	})

	cronScheduler.Start()
	log.Info().Msg("⏰ Cron scheduler started")

	// Start server in goroutine
	port := cfg.Port
	if port == "" {
		port = "3001"
	}

	go func() {
		log.Info().Msgf("🌐 Server listening on :%s", port)
		if err := app.Listen(":" + port); err != nil {
			log.Fatal().Err(err).Msg("Failed to start server")
		}
	}()

	// Run initial sync
	log.Info().Msg("🔄 Running initial sync...")
	if err := syncService.SyncMetrics(context.Background()); err != nil {
		log.Warn().Err(err).Msg("Initial metrics sync failed")
	}
	if err := syncService.SyncPools(context.Background()); err != nil {
		log.Warn().Err(err).Msg("Initial pools sync failed")
	}

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("🛑 Shutting down server...")
	cronScheduler.Stop()
	if err := app.Shutdown(); err != nil {
		log.Error().Err(err).Msg("Server shutdown error")
	}

	log.Info().Msg("✅ Server stopped")
}
