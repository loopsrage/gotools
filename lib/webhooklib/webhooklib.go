package webhooklib

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Event represents an outbound webhook delivery job.
type Event struct {
	ID          string            `json:"id"`
	URL         string            `json:"url"`
	EventType   string            `json:"event_type"`
	Payload     map[string]any    `json:"payload"`
	Headers     map[string]string `json:"headers,omitempty"`
	Secret      string            `json:"-"`
	Attempts    int               `json:"attempts"`
	MaxAttempts int               `json:"max_attempts"`
	LastError   string            `json:"last_error,omitempty"`
	StatusCode  int               `json:"status_code,omitempty"`
	CreatedAt   string            `json:"created_at"`
	DeliveredAt string            `json:"delivered_at,omitempty"`
}

// WebhookQueue manages pending, delivered, and dead-lettered webhook events.
type WebhookQueue struct {
	mu         sync.Mutex
	pending    []Event
	delivered  []Event
	deadLetter []Event
	client     *http.Client
	baseDelay  time.Duration
}

// NewWebhookQueue creates a new WebhookQueue with an HTTP timeout and retry base delay.
func NewWebhookQueue(timeout, baseDelay time.Duration) *WebhookQueue {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if baseDelay <= 0 {
		baseDelay = 100 * time.Millisecond
	}
	return &WebhookQueue{
		client:    &http.Client{Timeout: timeout},
		baseDelay: baseDelay,
	}
}

// Enqueue adds an event to the pending queue.
func (q *WebhookQueue) Enqueue(ev Event) Event {
	if ev.MaxAttempts <= 0 {
		ev.MaxAttempts = 3
	}
	if ev.CreatedAt == "" {
		ev.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	q.mu.Lock()
	q.pending = append(q.pending, ev)
	q.mu.Unlock()
	return ev
}

// DeliverOne delivers a single event with exponential backoff retries, recording it
// into either delivered or deadLetter.
func (q *WebhookQueue) DeliverOne(ctx context.Context, ev Event) (Event, error) {
	if ev.MaxAttempts <= 0 {
		ev.MaxAttempts = 3
	}
	body, err := json.Marshal(ev.Payload)
	if err != nil {
		ev.LastError = err.Error()
		q.recordDeadLetter(ev)
		return ev, err
	}
	var lastErr error
	for attempt := 1; attempt <= ev.MaxAttempts; attempt++ {
		ev.Attempts = attempt
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, ev.URL, bytes.NewReader(body))
		if err != nil {
			lastErr = err
			break
		}
		req.Header.Set("Content-Type", "application/json")
		if ev.EventType != "" {
			req.Header.Set("X-Webhook-Event", ev.EventType)
		}
		if ev.Secret != "" {
			mac := hmac.New(sha256.New, []byte(ev.Secret))
			mac.Write(body)
			req.Header.Set("X-Webhook-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
		}
		for k, v := range ev.Headers {
			req.Header.Set(k, v)
		}
		resp, err := q.client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			ev.StatusCode = resp.StatusCode
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				ev.LastError = ""
				ev.DeliveredAt = time.Now().UTC().Format(time.RFC3339)
				q.recordDelivered(ev)
				return ev, nil
			}
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		ev.LastError = lastErr.Error()
		if attempt < ev.MaxAttempts {
			delay := q.baseDelay * time.Duration(1<<(attempt-1))
			select {
			case <-ctx.Done():
				ev.LastError = ctx.Err().Error()
				q.recordDeadLetter(ev)
				return ev, ctx.Err()
			case <-time.After(delay):
			}
		}
	}
	q.recordDeadLetter(ev)
	return ev, lastErr
}

// Flush drains all currently pending events, delivering each.
func (q *WebhookQueue) Flush(ctx context.Context) (delivered int, failed int) {
	q.mu.Lock()
	batch := q.pending
	q.pending = nil
	q.mu.Unlock()
	for _, ev := range batch {
		if _, err := q.DeliverOne(ctx, ev); err == nil {
			delivered++
		} else {
			failed++
		}
	}
	return delivered, failed
}

func (q *WebhookQueue) recordDelivered(ev Event) {
	q.mu.Lock()
	q.delivered = append(q.delivered, ev)
	q.mu.Unlock()
}

func (q *WebhookQueue) recordDeadLetter(ev Event) {
	q.mu.Lock()
	q.deadLetter = append(q.deadLetter, ev)
	q.mu.Unlock()
}

// DeadLetters returns a snapshot of dead-lettered events.
func (q *WebhookQueue) DeadLetters() []Event {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]Event, len(q.deadLetter))
	copy(out, q.deadLetter)
	return out
}

// Stats returns queue counts.
func (q *WebhookQueue) Stats() map[string]any {
	q.mu.Lock()
	defer q.mu.Unlock()
	return map[string]any{
		"pending":     len(q.pending),
		"delivered":   len(q.delivered),
		"dead_letter": len(q.deadLetter),
	}
}
