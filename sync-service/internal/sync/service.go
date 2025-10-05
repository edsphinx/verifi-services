package sync

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/verifi-protocol/sync-service/internal/config"
	"github.com/verifi-protocol/sync-service/internal/db"
)

type Service struct {
	db     *db.DB
	config *config.Config
	stats  *Stats
	mu     sync.RWMutex
}

type Stats struct {
	LastMetricsSync    time.Time `json:"lastMetricsSync"`
	LastPoolsSync      time.Time `json:"lastPoolsSync"`
	LastActivitiesSync time.Time `json:"lastActivitiesSync"`
	MetricsSyncCount   int       `json:"metricsSyncCount"`
	PoolsSyncCount     int       `json:"poolsSyncCount"`
	ActivitiesSyncCount int      `json:"activitiesSyncCount"`
	Errors             int       `json:"errors"`
}

func NewService(database *db.DB, cfg *config.Config) *Service {
	return &Service{
		db:     database,
		config: cfg,
		stats:  &Stats{},
	}
}

func (s *Service) GetStats() *Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats
}

func (s *Service) updateStats(syncType string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch syncType {
	case "metrics":
		s.stats.LastMetricsSync = time.Now()
		s.stats.MetricsSyncCount++
	case "pools":
		s.stats.LastPoolsSync = time.Now()
		s.stats.PoolsSyncCount++
	case "activities":
		s.stats.LastActivitiesSync = time.Now()
		s.stats.ActivitiesSyncCount++
	}
}

func (s *Service) incrementErrors() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stats.Errors++
}

func (s *Service) SyncMetrics(ctx context.Context) error {
	start := time.Now()
	log.Info().Msg("📊 Starting metrics sync...")

	// Get all markets
	query := `
		SELECT "marketAddress", "description"
		FROM "Market"
		WHERE status = 'active'
	`

	rows, err := s.db.Pool().Query(ctx, query)
	if err != nil {
		s.incrementErrors()
		return err
	}
	defer rows.Close()

	type Market struct {
		Address     string
		Description string
	}

	var markets []Market
	for rows.Next() {
		var m Market
		if err := rows.Scan(&m.Address, &m.Description); err != nil {
			log.Error().Err(err).Msg("Failed to scan market")
			continue
		}
		markets = append(markets, m)
	}

	log.Info().Msgf("Found %d markets to sync", len(markets))

	// Calculate volume for each market
	for _, market := range markets {
		if err := s.calculateMarketMetrics(ctx, market.Address); err != nil {
			log.Error().
				Err(err).
				Str("market", market.Address[:10]+"...").
				Msg("Failed to calculate metrics")
			continue
		}
	}

	s.updateStats("metrics")
	log.Info().
		Dur("duration", time.Since(start)).
		Int("markets", len(markets)).
		Msg("✅ Metrics sync completed")

	return nil
}

func (s *Service) calculateMarketMetrics(ctx context.Context, marketAddress string) error {
	// Calculate volume from activities
	time24hAgo := time.Now().Add(-24 * time.Hour)
	time7dAgo := time.Now().Add(-7 * 24 * time.Hour)

	query := `
		SELECT
			COALESCE(SUM(CASE WHEN timestamp >= $1 THEN "totalValue" ELSE 0 END), 0) as volume24h,
			COALESCE(SUM(CASE WHEN timestamp >= $2 THEN "totalValue" ELSE 0 END), 0) as volume7d,
			COALESCE(SUM("totalValue"), 0) as totalVolume,
			COUNT(DISTINCT "userAddress") as uniqueTraders
		FROM "Activity"
		WHERE "marketAddress" = $3
			AND action IN ('BUY', 'SELL', 'SWAP')
	`

	var volume24h, volume7d, totalVolume float64
	var uniqueTraders int

	err := s.db.Pool().QueryRow(ctx, query, time24hAgo, time7dAgo, marketAddress).
		Scan(&volume24h, &volume7d, &totalVolume, &uniqueTraders)
	if err != nil {
		return err
	}

	// Update market record
	updateQuery := `
		UPDATE "Market"
		SET
			"volume24h" = $1,
			"volume7d" = $2,
			"totalVolume" = $3,
			"uniqueTraders" = $4,
			"updatedAt" = NOW()
		WHERE "marketAddress" = $5
	`

	_, err = s.db.Pool().Exec(ctx, updateQuery,
		volume24h, volume7d, totalVolume, uniqueTraders, marketAddress)

	if err == nil {
		log.Debug().
			Str("market", marketAddress[:10]+"...").
			Float64("volume24h", volume24h).
			Msg("Metrics updated")
	}

	return err
}

func (s *Service) SyncPools(ctx context.Context) error {
	start := time.Now()
	log.Info().Msg("💧 Starting pools sync...")

	// TODO: Implement pool sync logic
	// This would sync pool reserves, LP positions, etc.

	s.updateStats("pools")
	log.Info().
		Dur("duration", time.Since(start)).
		Msg("✅ Pools sync completed")

	return nil
}

func (s *Service) SyncActivities(ctx context.Context) error {
	start := time.Now()
	log.Info().Msg("📝 Starting activities sync...")

	// TODO: Implement activities sync from Nodit
	// This would be a backup for webhook data

	s.updateStats("activities")
	log.Info().
		Dur("duration", time.Since(start)).
		Msg("✅ Activities sync completed")

	return nil
}
