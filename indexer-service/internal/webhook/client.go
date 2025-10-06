package webhook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

type WebhookClient struct {
	URL    string
	Client *http.Client
}

type WebhookPayload struct {
	Event       EventData       `json:"event"`
	Transaction TransactionData `json:"transaction"`
}

type EventData struct {
	Type string                 `json:"type"`
	Data map[string]interface{} `json:"data"`
}

type TransactionData struct {
	Hash      string `json:"hash"`
	Sender    string `json:"sender"`
	Timestamp string `json:"timestamp"`
}

func NewWebhookClient(url string) *WebhookClient {
	return &WebhookClient{
		URL: url,
		Client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (w *WebhookClient) SendEvent(eventType string, eventData map[string]interface{}, txHash string, sender string) error {
	payload := WebhookPayload{
		Event: EventData{
			Type: eventType,
			Data: eventData,
		},
		Transaction: TransactionData{
			Hash:      txHash,
			Sender:    sender,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	log.Printf("🔔 Sending webhook to %s for event %s", w.URL, eventType)

	req, err := http.NewRequest("POST", w.URL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := w.Client.Do(req)
	if err != nil {
		log.Printf("⚠️  Webhook request failed (non-critical): %v", err)
		return nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		log.Printf("✅ Webhook delivered successfully: %s", string(body))
	} else {
		log.Printf("⚠️  Webhook returned non-success status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
