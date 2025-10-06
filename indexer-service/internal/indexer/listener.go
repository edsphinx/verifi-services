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

	log.Info().
		Str("webhook_url", webhookURL).
		Bool("is_empty", webhookURL == "").
		Msg("🔧 Initializing EventListener with webhook config")

	if webhookURL != "" {
		webhookClient = webhook.NewWebhookClient(webhookURL)
		log.Info().Str("webhook_url", webhookURL).Msg("📡 Webhook client initialized successfully")
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
	log.Debug().
		Uint64("current_version", l.lastVersion).
		Msg("🔄 Starting poll cycle")

	// Get latest version
	latestVersion, err := l.client.GetLatestLedgerInfo(ctx)
	if err != nil {
		log.Error().Err(err).Msg("❌ Failed to get latest ledger info")
		return err
	}

	log.Debug().
		Uint64("latest_version", latestVersion).
		Uint64("last_processed", l.lastVersion).
		Uint64("diff", latestVersion-l.lastVersion).
		Msg("📊 Ledger info retrieved")

	// No new transactions
	if latestVersion <= l.lastVersion {
		log.Debug().Msg("⏸️  No new transactions to process")
		return nil
	}

	log.Info().
		Uint64("from", l.lastVersion+1).
		Uint64("to", latestVersion).
		Uint64("count", latestVersion-l.lastVersion).
		Msg("📥 Processing new transactions")

	// Fetch transactions in batches
	batchSize := uint64(100)
	start := l.lastVersion + 1
	end := latestVersion

	for start <= end {
		limit := batchSize
		if start+limit > end {
			limit = end - start + 1
		}

		log.Debug().
			Uint64("start", start).
			Uint64("limit", limit).
			Msg("🔍 Fetching transaction batch")

		txs, err := l.client.GetTransactionsByVersionRange(ctx, start, limit)
		if err != nil {
			log.Error().
				Err(err).
				Uint64("start", start).
				Uint64("limit", limit).
				Msg("❌ Failed to fetch transactions")
			return err
		}

		log.Debug().
			Int("tx_count", len(txs)).
			Msg("✅ Transactions fetched")

		// Process each transaction
		for _, tx := range txs {
			if err := l.processTx(ctx, tx); err != nil {
				log.Error().
					Err(err).
					Str("version", tx.Version).
					Str("hash", tx.Hash).
					Msg("❌ Failed to process transaction")
				continue
			}
		}

		start += limit
	}

	// Update last version
	l.lastVersion = latestVersion
	log.Info().
		Uint64("new_version", latestVersion).
		Msg("💾 Updating last processed version")

	if err := l.saveLastVersion(ctx); err != nil {
		log.Error().Err(err).Msg("❌ Failed to save last version")
	}

	return nil
}

func (l *EventListener) processTx(ctx context.Context, tx TransactionEvent) error {
	// Only process successful user transactions
	if !tx.Success || tx.Type != "user_transaction" {
		log.Debug().
			Str("type", tx.Type).
			Bool("success", tx.Success).
			Msg("⏭️  Skipping non-user or failed transaction")
		return nil
	}

	log.Debug().
		Str("hash", tx.Hash).
		Int("event_count", len(tx.Events)).
		Msg("🔍 Processing user transaction")

	// Process each event in the transaction
	for _, event := range tx.Events {
		log.Debug().
			Str("event_type", event.Type).
			Str("module_address", l.moduleAddress).
			Bool("contains_module", strings.Contains(event.Type, l.moduleAddress)).
			Msg("📝 Checking event")

		// Check if event is from our module
		if !strings.Contains(event.Type, l.moduleAddress) {
			continue
		}

		log.Info().
			Str("event_type", event.Type).
			Msg("✅ Found event from our module")

		// Extract event name
		parts := strings.Split(event.Type, "::")
		if len(parts) < 3 {
			log.Warn().
				Str("event_type", event.Type).
				Int("parts", len(parts)).
				Msg("⚠️  Event type has unexpected format")
			continue
		}
		eventName := parts[len(parts)-1]

		log.Info().
			Str("event_name", eventName).
			Msg("🎯 Extracted event name")

		// Find handler
		handler, exists := l.eventHandlers[eventName]
		if !exists {
			log.Debug().
				Str("event", eventName).
				Interface("available_handlers", l.getHandlerNames()).
				Msg("⚠️  No handler registered for event")
			continue
		}

		log.Info().
			Str("event", eventName).
			Msg("▶️  Executing handler")

		// Execute handler
		if err := handler(ctx, event, tx); err != nil {
			log.Error().
				Err(err).
				Str("event", eventName).
				Str("tx", tx.Hash).
				Msg("❌ Handler error")
		}
	}

	return nil
}

// Helper to get registered handler names for debugging
func (l *EventListener) getHandlerNames() []string {
	names := make([]string, 0, len(l.eventHandlers))
	for name := range l.eventHandlers {
		names = append(names, name)
	}
	return names
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
		Str("event_type", event.Type).
		Msg("🎯 MarketCreatedEvent detected")

	// Log raw event data for debugging
	log.Debug().
		Interface("event_data", event.Data).
		Msg("📦 Raw event data")

	// Extract event data - use correct field names from Move struct
	marketAddress, okAddr := event.Data["market_address"].(string)
	creator, okCreator := event.Data["creator"].(string)
	description, okDesc := event.Data["description"].(string)
	resolutionTimestamp, okRes := event.Data["resolution_timestamp"].(string)

	log.Info().
		Str("market", marketAddress).
		Str("creator", creator).
		Str("description", description).
		Str("resolution_timestamp", resolutionTimestamp).
		Bool("addr_ok", okAddr).
		Bool("creator_ok", okCreator).
		Bool("desc_ok", okDesc).
		Bool("res_ok", okRes).
		Msg("✅ Extracted market data")

	// Trigger webhook for live notifications
	if l.webhookClient != nil {
		log.Info().Msg("🔔 Webhook client exists, preparing to send...")

		eventData := make(map[string]interface{})
		eventData["market_address"] = marketAddress
		eventData["creator"] = creator
		eventData["description"] = description
		eventData["resolution_timestamp"] = resolutionTimestamp

		log.Info().
			Interface("event_data", eventData).
			Str("webhook_url", l.webhookClient.URL).
			Msg("📤 Sending webhook with data")

		err := l.webhookClient.SendEvent(event.Type, eventData, tx.Hash, tx.Sender)
		if err != nil {
			log.Error().Err(err).Msg("❌ Webhook trigger failed")
		} else {
			log.Info().Msg("✅ Webhook sent successfully")
		}
	} else {
		log.Warn().Msg("⚠️  Webhook client is nil, skipping webhook notification")
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
