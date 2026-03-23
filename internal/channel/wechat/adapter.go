package wechat

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/xavierli/nethelper/internal/channel"
)

// Adapter connects to a WeChat bridge service (e.g. iLink Bot, nexu)
// via HTTP long-polling. WeChat has no official bot API; this adapter
// delegates all WeChat I/O to a separately deployed bridge process.
type Adapter struct {
	bridgeURL string // e.g. "http://localhost:9000"
	token     string // optional auth token for the bridge
	handler   channel.MessageHandler
	client    *http.Client
	ctx       context.Context
	cancel    context.CancelFunc
}

// New creates a new WeChat adapter pointed at the given bridge URL.
func New(bridgeURL, token string) *Adapter {
	return &Adapter{
		bridgeURL: strings.TrimRight(bridgeURL, "/"),
		token:     token,
		client:    &http.Client{Timeout: 35 * time.Second},
	}
}

// Name returns the channel name.
func (a *Adapter) Name() string { return "wechat" }

// Start begins the HTTP long-polling loop and blocks until the context is
// cancelled. Transient poll errors are logged and retried after 5 seconds.
func (a *Adapter) Start(ctx context.Context, handler channel.MessageHandler) error {
	a.handler = handler
	a.ctx, a.cancel = context.WithCancel(ctx)

	for {
		select {
		case <-a.ctx.Done():
			return nil
		default:
		}

		messages, err := a.poll()
		if err != nil {
			log.Printf("[wechat] poll error: %v", err)
			// Back off briefly before retrying.
			select {
			case <-a.ctx.Done():
				return nil
			case <-time.After(5 * time.Second):
			}
			continue
		}

		for _, msg := range messages {
			if a.handler != nil {
				a.handler(msg)
			}
		}
	}
}

// Stop cancels the context, unblocking the polling loop.
func (a *Adapter) Stop() error {
	if a.cancel != nil {
		a.cancel()
	}
	return nil
}

// SendText sends a plain-text message to the given chat ID via the bridge.
func (a *Adapter) SendText(chatID, text string) error {
	payload, _ := json.Marshal(map[string]string{
		"chat_id": chatID,
		"text":    text,
	})
	req, err := http.NewRequestWithContext(a.ctx, http.MethodPost,
		a.bridgeURL+"/send", strings.NewReader(string(payload)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if a.token != "" {
		req.Header.Set("Authorization", "Bearer "+a.token)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("wechat send %d: %s", resp.StatusCode, body)
	}
	return nil
}

// bridgeMessage is the JSON shape returned by the bridge /poll endpoint.
type bridgeMessage struct {
	ChatID   string `json:"chat_id"`
	UserID   string `json:"user_id"`
	UserName string `json:"user_name"`
	Text     string `json:"text"`
	IsGroup  bool   `json:"is_group"`
}

// poll performs one long-poll request to the bridge and returns any waiting
// messages. Returns nil, nil when the bridge signals no messages (HTTP 204).
func (a *Adapter) poll() ([]channel.InMessage, error) {
	req, err := http.NewRequestWithContext(a.ctx, http.MethodGet,
		a.bridgeURL+"/poll", nil)
	if err != nil {
		return nil, err
	}
	if a.token != "" {
		req.Header.Set("Authorization", "Bearer "+a.token)
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNoContent {
		return nil, nil // no messages this cycle
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("poll %d: %s", resp.StatusCode, body)
	}

	var msgs []bridgeMessage
	if err := json.NewDecoder(resp.Body).Decode(&msgs); err != nil {
		return nil, err
	}

	result := make([]channel.InMessage, 0, len(msgs))
	for _, m := range msgs {
		result = append(result, channel.InMessage{
			Channel:  "wechat",
			ChatID:   m.ChatID,
			UserID:   m.UserID,
			UserName: m.UserName,
			Text:     m.Text,
			IsGroup:  m.IsGroup,
		})
	}
	return result, nil
}
