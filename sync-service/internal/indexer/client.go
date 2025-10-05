package indexer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

const (
	AptosTestnetRPC = "https://fullnode.testnet.aptoslabs.com/v1"
	AptosMainnetRPC = "https://fullnode.mainnet.aptoslabs.com/v1"
)

type Client struct {
	rpcURL     string
	httpClient *http.Client
}

func NewClient(network string) *Client {
	rpcURL := AptosTestnetRPC
	if network == "mainnet" {
		rpcURL = AptosMainnetRPC
	}

	return &Client{
		rpcURL: rpcURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

type EventQuery struct {
	EventType string
	Start     uint64
	Limit     int
}

type Event struct {
	Version         string                 `json:"version"`
	GUID            map[string]interface{} `json:"guid"`
	SequenceNumber  string                 `json:"sequence_number"`
	Type            string                 `json:"type"`
	Data            map[string]interface{} `json:"data"`
}

type TransactionEvent struct {
	Version         string                 `json:"version"`
	Hash            string                 `json:"hash"`
	StateChangeHash string                 `json:"state_change_hash"`
	EventRootHash   string                 `json:"event_root_hash"`
	GasUsed         string                 `json:"gas_used"`
	Success         bool                   `json:"success"`
	VMStatus        string                 `json:"vm_status"`
	AccumulatorRootHash string             `json:"accumulator_root_hash"`
	Changes         []interface{}          `json:"changes"`
	Events          []Event                `json:"events"`
	Timestamp       string                 `json:"timestamp"`
	Type            string                 `json:"type"`
}

// Get events by event handle
func (c *Client) GetEventsByEventHandle(ctx context.Context, address, eventHandle, fieldName string, start, limit uint64) ([]Event, error) {
	url := fmt.Sprintf("%s/accounts/%s/events/%s/%s?start=%d&limit=%d",
		c.rpcURL, address, eventHandle, fieldName, start, limit)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("RPC error: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var events []Event
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		return nil, err
	}

	return events, nil
}

// Get transactions by version range
func (c *Client) GetTransactionsByVersionRange(ctx context.Context, start, limit uint64) ([]TransactionEvent, error) {
	url := fmt.Sprintf("%s/transactions?start=%d&limit=%d", c.rpcURL, start, limit)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("RPC error: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var txs []TransactionEvent
	if err := json.NewDecoder(resp.Body).Decode(&txs); err != nil {
		return nil, err
	}

	return txs, nil
}

// Get latest ledger info
func (c *Client) GetLatestLedgerInfo(ctx context.Context) (uint64, error) {
	url := fmt.Sprintf("%s", c.rpcURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	var result struct {
		LedgerVersion string `json:"ledger_version"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}

	var version uint64
	fmt.Sscanf(result.LedgerVersion, "%d", &version)
	return version, nil
}

// View function call
func (c *Client) View(ctx context.Context, function string, typeArgs, args []string) ([]interface{}, error) {
	type ViewRequest struct {
		Function      string   `json:"function"`
		TypeArguments []string `json:"type_arguments"`
		Arguments     []string `json:"arguments"`
	}

	reqBody := ViewRequest{
		Function:      function,
		TypeArguments: typeArgs,
		Arguments:     args,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/view", c.rpcURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("view call error: status=%d, body=%s", resp.StatusCode, string(body))
	}

	var result []interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result, nil
}
