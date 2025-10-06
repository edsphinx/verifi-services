package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	DatabaseURL    string
	AptosNetwork   string
	ModuleAddress  string
	Port           string
	WebhookURL     string
	AptosAPIKeys   []string
	NoditAPIKeys   []string
}

func Load() (*Config, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}

	network := os.Getenv("NEXT_PUBLIC_APTOS_NETWORK")
	if network == "" {
		network = "testnet"
	}

	moduleAddr := os.Getenv("NEXT_PUBLIC_PUBLISHER_ACCOUNT_ADDRESS")
	if moduleAddr == "" {
		return nil, fmt.Errorf("NEXT_PUBLIC_PUBLISHER_ACCOUNT_ADDRESS is required")
	}

	port := os.Getenv("INDEXER_PORT")
	if port == "" {
		port = "3002"
	}

	// Load webhook URL (optional)
	webhookURL := os.Getenv("WEBHOOK_URL")

	// Load API keys (comma-separated)
	aptosKeys := []string{}
	if aptosKeysStr := os.Getenv("APTOS_API_KEYS"); aptosKeysStr != "" {
		aptosKeys = strings.Split(aptosKeysStr, ",")
		// Trim spaces
		for i := range aptosKeys {
			aptosKeys[i] = strings.TrimSpace(aptosKeys[i])
		}
	}

	noditKeys := []string{}
	if noditKeysStr := os.Getenv("NODIT_API_KEYS"); noditKeysStr != "" {
		noditKeys = strings.Split(noditKeysStr, ",")
		// Trim spaces
		for i := range noditKeys {
			noditKeys[i] = strings.TrimSpace(noditKeys[i])
		}
	}

	return &Config{
		DatabaseURL:   dbURL,
		AptosNetwork:  network,
		ModuleAddress: moduleAddr,
		Port:          port,
		WebhookURL:    webhookURL,
		AptosAPIKeys:  aptosKeys,
		NoditAPIKeys:  noditKeys,
	}, nil
}
