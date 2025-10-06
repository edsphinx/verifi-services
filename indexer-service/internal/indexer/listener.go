package indexer

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/verifi-protocol/indexer-service/internal/db"
	"github.com/verifi-protocol/indexer-service/internal/webhook"
)

type EventListener struct {
	client          *Client
	db              *db.DB
	moduleAddress   string
	lastVersion     uint64
	pollInterval    time.Duration
	eventHandlers   map[string]EventHandler
	webhookClient   *webhook.WebhookClient
}

func (l *EventListener) GetLastVersion() uint64 {
	return l.lastVersion
}

type EventHandler func(ctx context.Context, event Event, tx TransactionEvent) error

func NewEventListener(client *Client, database *db.DB, moduleAddress string, webhookURL string) *EventListener {
	var webhookClient *webhook.WebhookClient
	if webhookURL != "" {
		webhookClient = webhook.NewWebhookClient(webhookURL)
		log.Info().Str("webhook_url", webhookURL).Msg("📡 Webhook client initialized")
	} else {
		log.Warn().Msg("⚠️  No webhook URL provided, notifications will not be sent")
	}

	return &EventListener{
		client:        client,
		db:            database,
		moduleAddress: moduleAddress,
		pollInterval:  5 * time.Second, // Poll every 5 seconds
		eventHandlers: make(map[string]EventHandler),
		webhookClient: webhookClient,
	}
}

// Register event handlers
func (l *EventListener) RegisterHandler(eventType string, handler EventHandler) {
	l.eventHandlers[eventType] = handler
}

// Start listening for events
func (l *EventListener) Start(ctx context.Context) error {
	log.Info().Msg("🎧 Starting event listener...")

	// Get last processed version from DB
	if err := l.loadLastVersion(ctx); err != nil {
		log.Warn().Err(err).Msg("Failed to load last version, starting from latest")
		// Start from current version
		version, err := l.client.GetLatestLedgerInfo(ctx)
		if err != nil {
			return fmt.Errorf("failed to get latest ledger info: %w", err)
		}
		l.lastVersion = version
	}

	log.Info().Uint64("version", l.lastVersion).Msg("Starting from version")

	// Register default handlers
	l.registerDefaultHandlers()

	// Start polling loop
	ticker := time.NewTicker(l.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Event listener stopped")
			return nil
		case <-ticker.C:
			if err := l.poll(ctx); err != nil {
				log.Error().Err(err).Msg("Polling error")
			}
		}
	}
}

func (l *EventListener) poll(ctx context.Context) error {
	// Get latest version
	latestVersion, err := l.client.GetLatestLedgerInfo(ctx)
	if err != nil {
		return err
	}

	// No new transactions
	if latestVersion <= l.lastVersion {
		return nil
	}

	// Fetch transactions in batches
	batchSize := uint64(100)
	start := l.lastVersion + 1
	end := latestVersion

	for start <= end {
		limit := batchSize
		if start+limit > end {
			limit = end - start + 1
		}

		txs, err := l.client.GetTransactionsByVersionRange(ctx, start, limit)
		if err != nil {
			return err
		}

		// Process each transaction
		for _, tx := range txs {
			if err := l.processTx(ctx, tx); err != nil {
				log.Error().
					Err(err).
					Str("version", tx.Version).
					Str("hash", tx.Hash).
					Msg("Failed to process transaction")
				continue
			}
		}

		start += limit
	}

	// Update last version
	l.lastVersion = latestVersion
	if err := l.saveLastVersion(ctx); err != nil {
		log.Error().Err(err).Msg("Failed to save last version")
	}

	return nil
}

func (l *EventListener) processTx(ctx context.Context, tx TransactionEvent) error {
	// Only process successful user transactions
	if !tx.Success || tx.Type != "user_transaction" {
		return nil
	}

	// Process each event in the transaction
	for _, event := range tx.Events {
		// Check if event is from our module
		if !strings.Contains(event.Type, l.moduleAddress) {
			continue
		}

		// Extract event name
		parts := strings.Split(event.Type, "::")
		if len(parts) < 3 {
			continue
		}
		eventName := parts[len(parts)-1]

		// Find handler
		handler, exists := l.eventHandlers[eventName]
		if !exists {
			log.Debug().Str("event", eventName).Msg("No handler registered")
			continue
		}

		// Execute handler
		if err := handler(ctx, event, tx); err != nil {
			log.Error().
				Err(err).
				Str("event", eventName).
				Str("tx", tx.Hash).
				Msg("Handler error")
		}
	}

	return nil
}

func (l *EventListener) registerDefaultHandlers() {
	// SharesMintedEvent - when user buys shares
	l.RegisterHandler("SharesMintedEvent", l.handleSharesMinted)

	// SharesBurnedEvent - when user sells shares
	l.RegisterHandler("SharesBurnedEvent", l.handleSharesBurned)

	// MarketCreatedEvent - when new market is created
	l.RegisterHandler("MarketCreatedEvent", l.handleMarketCreated)

	// MarketResolvedEvent - when market is resolved
	l.RegisterHandler("MarketResolvedEvent", l.handleMarketResolved)
}

func (l *EventListener) handleSharesMinted(ctx context.Context, event Event, tx TransactionEvent) error {
	log.Info().
		Str("tx", tx.Hash).
		Msg("📈 SharesMintedEvent detected")

	// Extract event data
	marketAddress, _ := event.Data["market_address"].(string)
	user, _ := event.Data["user"].(string)
	aptAmountIn, _ := event.Data["apt_amount_in"].(string)
	sharesOut, _ := event.Data["shares_out"].(string)
	isYes, _ := event.Data["is_yes"].(bool)

	// Convert amounts
	aptAmount, _ := strconv.ParseFloat(aptAmountIn, 64)
	aptAmount = aptAmount / 1e8 // Convert from octas

	shares, _ := strconv.ParseFloat(sharesOut, 64)
	shares = shares / 1e6 // Convert from token decimals

	outcome := "NO"
	if isYes {
		outcome = "YES"
	}

	// Insert activity record
	query := `
		INSERT INTO "Activity" (
			"id", "txHash", "marketAddress", "userAddress",
			"action", "outcome", "amount", "totalValue", "timestamp"
		) VALUES (
			gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8
		)
		ON CONFLICT ("txHash") DO NOTHING
	`

	timestamp, _ := time.Parse(time.RFC3339, tx.Timestamp)

	_, err := l.db.Pool().Exec(ctx, query,
		tx.Hash,
		marketAddress,
		user,
		"BUY",
		outcome,
		shares,
		aptAmount,
		timestamp,
	)

	if err != nil {
		return fmt.Errorf("failed to insert activity: %w", err)
	}

	log.Info().
		Str("market", marketAddress[:10]+"...").
		Str("user", user[:10]+"...").
		Float64("apt", aptAmount).
		Float64("shares", shares).
		Str("outcome", outcome).
		Msg("✅ BUY activity recorded")

	// Trigger webhook for live notifications
	if l.webhookClient != nil {
		eventData := make(map[string]interface{})
		eventData["market_address"] = marketAddress
		eventData["buyer"] = user
		eventData["is_yes_outcome"] = isYes
		eventData["apt_amount_in"] = aptAmountIn
		eventData["shares_out"] = sharesOut

		err := l.webhookClient.SendEvent(event.Type, eventData, tx.Hash, tx.Sender)
		if err != nil {
			log.Warn().Err(err).Msg("Webhook trigger failed (non-critical)")
		}
	}

	return nil
}

func (l *EventListener) handleSharesBurned(ctx context.Context, event Event, tx TransactionEvent) error {
	log.Info().
		Str("tx", tx.Hash).
		Msg("📉 SharesBurnedEvent detected")

	marketAddress, _ := event.Data["market_address"].(string)
	user, _ := event.Data["user"].(string)
	sharesIn, _ := event.Data["shares_in"].(string)
	aptAmountOut, _ := event.Data["apt_amount_out"].(string)
	isYes, _ := event.Data["is_yes"].(bool)

	aptAmount, _ := strconv.ParseFloat(aptAmountOut, 64)
	aptAmount = aptAmount / 1e8

	shares, _ := strconv.ParseFloat(sharesIn, 64)
	shares = shares / 1e6

	outcome := "NO"
	if isYes {
		outcome = "YES"
	}

	query := `
		INSERT INTO "Activity" (
			"id", "txHash", "marketAddress", "userAddress",
			"action", "outcome", "amount", "totalValue", "timestamp"
		) VALUES (
			gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8
		)
		ON CONFLICT ("txHash") DO NOTHING
	`

	timestamp, _ := time.Parse(time.RFC3339, tx.Timestamp)

	_, err := l.db.Pool().Exec(ctx, query,
		tx.Hash,
		marketAddress,
		user,
		"SELL",
		outcome,
		shares,
		aptAmount,
		timestamp,
	)

	if err != nil {
		return fmt.Errorf("failed to insert activity: %w", err)
	}

	log.Info().
		Str("market", marketAddress[:10]+"...").
		Str("user", user[:10]+"...").
		Float64("apt", aptAmount).
		Float64("shares", shares).
		Str("outcome", outcome).
		Msg("✅ SELL activity recorded")

	// Trigger webhook for live notifications
	if l.webhookClient != nil {
		eventData := make(map[string]interface{})
		eventData["market_address"] = marketAddress
		eventData["seller"] = user
		eventData["is_yes_outcome"] = isYes
		eventData["apt_amount_out"] = aptAmountOut
		eventData["shares_in"] = sharesIn

		err := l.webhookClient.SendEvent(event.Type, eventData, tx.Hash, tx.Sender)
		if err != nil {
			log.Warn().Err(err).Msg("Webhook trigger failed (non-critical)")
		}
	}

	return nil
}

func (l *EventListener) handleMarketCreated(ctx context.Context, event Event, tx TransactionEvent) error {
	log.Info().
		Str("tx", tx.Hash).
		Msg("🎯 MarketCreatedEvent detected")

	// Extract event data - use correct field names from Move struct
	marketAddress, _ := event.Data["market_address"].(string)
	creator, _ := event.Data["creator"].(string)
	description, _ := event.Data["description"].(string)
	resolutionTimestamp, _ := event.Data["resolution_timestamp"].(string)

	log.Info().
		Str("market", marketAddress[:10]+"...").
		Str("creator", creator[:10]+"...").
		Str("description", description).
		Msg("✅ New market created")

	// Trigger webhook for live notifications
	if l.webhookClient != nil {
		eventData := make(map[string]interface{})
		eventData["market_address"] = marketAddress
		eventData["creator"] = creator
		eventData["description"] = description
		eventData["resolution_timestamp"] = resolutionTimestamp

		err := l.webhookClient.SendEvent(event.Type, eventData, tx.Hash, tx.Sender)
		if err != nil {
			log.Warn().Err(err).Msg("Webhook trigger failed (non-critical)")
		}
	}

	return nil
}

func (l *EventListener) handleMarketResolved(ctx context.Context, event Event, tx TransactionEvent) error {
	log.Info().
		Str("tx", tx.Hash).
		Msg("✅ MarketResolvedEvent detected")

	marketAddress, _ := event.Data["market_address"].(string)
	outcome, _ := event.Data["outcome"].(string)

	log.Info().
		Str("market", marketAddress[:10]+"...").
		Str("outcome", outcome).
		Msg("🏁 Market resolved")

	// Update market status in DB
	query := `
		UPDATE "Market"
		SET status = $1, "updatedAt" = NOW()
		WHERE "marketAddress" = $2
	`

	_, err := l.db.Pool().Exec(ctx, query, "resolved", marketAddress)
	if err != nil {
		return fmt.Errorf("failed to update market status: %w", err)
	}

	return nil
}

func (l *EventListener) loadLastVersion(ctx context.Context) error {
	query := `
		SELECT value FROM sync_state WHERE key = 'last_indexed_version'
	`

	var versionStr string
	err := l.db.Pool().QueryRow(ctx, query).Scan(&versionStr)
	if err != nil {
		return err
	}

	version, err := strconv.ParseUint(versionStr, 10, 64)
	if err != nil {
		return err
	}

	l.lastVersion = version
	return nil
}

func (l *EventListener) saveLastVersion(ctx context.Context) error {
	query := `
		INSERT INTO sync_state (key, value, updated_at)
		VALUES ('last_indexed_version', $1, NOW())
		ON CONFLICT (key) DO UPDATE SET value = $1, updated_at = NOW()
	`

	_, err := l.db.Pool().Exec(ctx, query, strconv.FormatUint(l.lastVersion, 10))
	return err
}
