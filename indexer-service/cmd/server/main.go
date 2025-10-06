package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/verifi-protocol/indexer-service/internal/config"
	"github.com/verifi-protocol/indexer-service/internal/db"
	"github.com/verifi-protocol/indexer-service/internal/indexer"
	"github.com/verifi-protocol/indexer-service/internal/logbuffer"
)

func main() {
	// Load environment variables from main project
	if err := godotenv.Load("../.env"); err != nil {
		if err := godotenv.Load("../.env.local"); err != nil {
			log.Warn().Msg("No .env file found in parent directory, using system environment variables")
		}
	}

	// Setup logger
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	// Initialize log buffer for HTTP endpoint
	logbuffer.Init(500) // Keep last 500 log entries

	// Create multi-writer: console + log buffer
	consoleWriter := zerolog.ConsoleWriter{Out: os.Stderr}
	multiWriter := zerolog.MultiLevelWriter(
		consoleWriter,
		&logBufferWriter{},
	)
	log.Logger = log.Output(multiWriter)

	log.Info().Msg("🎧 VeriFi Event Indexer Starting...")

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

	// Run migrations
	if err := runMigrations(database); err != nil {
		log.Fatal().Err(err).Msg("Failed to run migrations")
	}

	// Initialize Aptos client
	aptosClient := indexer.NewClient(cfg.AptosNetwork)
	log.Info().Str("network", cfg.AptosNetwork).Msg("✅ Aptos client initialized")

	// Initialize API key rotator if keys are provided
	if len(cfg.AptosAPIKeys) > 0 || len(cfg.NoditAPIKeys) > 0 {
		rotator := indexer.NewAPIKeyRotator(cfg.AptosAPIKeys, cfg.NoditAPIKeys)
		aptosClient.SetAPIRotator(rotator)
		log.Info().
			Int("aptos_keys", len(cfg.AptosAPIKeys)).
			Int("nodit_keys", len(cfg.NoditAPIKeys)).
			Msg("✅ API key rotation enabled")
	}

	// Initialize event listener
	listener := indexer.NewEventListener(aptosClient, database, cfg.ModuleAddress, cfg.WebhookURL)

	// Setup Fiber app
	app := fiber.New(fiber.Config{
		AppName:      "VeriFi Event Indexer",
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
			"service": "verifi-indexer-service",
			"time":    time.Now().Unix(),
		})
	})

	// Status endpoint
	app.Get("/status", func(c *fiber.Ctx) error {
		version := listener.GetLastVersion()
		return c.JSON(fiber.Map{
			"status":       "running",
			"last_version": version,
			"network":      cfg.AptosNetwork,
		})
	})

	// Logs endpoint - returns recent logs
	app.Get("/logs", func(c *fiber.Ctx) error {
		// Get limit from query param, default 100
		limit := c.QueryInt("limit", 100)
		if limit > 500 {
			limit = 500
		}

		logs := logbuffer.GetRecent(limit)
		return c.JSON(fiber.Map{
			"logs":  logs,
			"count": len(logs),
		})
	})

	// Start server in goroutine
	go func() {
		log.Info().Msgf("🌐 Server listening on :%s", cfg.Port)
		if err := app.Listen(":" + cfg.Port); err != nil {
			log.Fatal().Err(err).Msg("Failed to start server")
		}
	}()

	// Start event listener in goroutine
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := listener.Start(ctx); err != nil {
			log.Error().Err(err).Msg("Event listener error")
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("🛑 Shutting down indexer...")
	cancel() // Stop event listener

	if err := app.Shutdown(); err != nil {
		log.Error().Err(err).Msg("Server shutdown error")
	}

	log.Info().Msg("✅ Indexer stopped")
}

func runMigrations(database *db.DB) error {
	log.Info().Msg("🔄 Running migrations...")

	migration := `
	CREATE TABLE IF NOT EXISTS sync_state (
		key VARCHAR(255) PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at TIMESTAMP DEFAULT NOW()
	);

	INSERT INTO sync_state (key, value, updated_at)
	VALUES ('last_indexed_version', '0', NOW())
	ON CONFLICT (key) DO NOTHING;
	`

	_, err := database.Pool().Exec(context.Background(), migration)
	if err != nil {
		return err
	}

	log.Info().Msg("✅ Migrations complete")
	return nil
}

// Custom writer to capture logs into buffer
type logBufferWriter struct{}

func (w *logBufferWriter) Write(p []byte) (n int, err error) {
	logbuffer.Add("INFO", string(p))
	return len(p), nil
}

func (w *logBufferWriter) WriteLevel(level zerolog.Level, p []byte) (n int, err error) {
	logbuffer.Add(level.String(), string(p))
	return len(p), nil
}
