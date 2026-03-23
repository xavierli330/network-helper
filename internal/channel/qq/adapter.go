package qq

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/gorilla/websocket"

	"github.com/xavierli/nethelper/internal/channel"
)

// Adapter connects to a go-cqhttp-compatible OneBot WebSocket server and
// implements channel.Channel for QQ messaging.
// ChatIDs for group chats are prefixed with "group:" to distinguish them
// from private (user) chat IDs when sending.
type Adapter struct {
	wsURL   string // e.g. "ws://localhost:6700"
	handler channel.MessageHandler
	conn    *websocket.Conn
	ctx     context.Context
	cancel  context.CancelFunc
	selfID  string
}

// New creates a new QQ adapter that will connect to the given OneBot WS URL.
func New(wsURL string) *Adapter {
	return &Adapter{wsURL: wsURL}
}

// Name returns the channel name.
func (a *Adapter) Name() string { return "qq" }

// Start connects to the OneBot WebSocket endpoint and begins reading events.
// It blocks until the context is cancelled or the connection is lost.
func (a *Adapter) Start(ctx context.Context, handler channel.MessageHandler) error {
	a.handler = handler
	a.ctx, a.cancel = context.WithCancel(ctx)

	var err error
	a.conn, _, err = websocket.DefaultDialer.DialContext(ctx, a.wsURL, nil)
	if err != nil {
		return fmt.Errorf("qq ws connect: %w", err)
	}

	for {
		select {
		case <-a.ctx.Done():
			a.conn.Close()
			return nil
		default:
		}

		_, data, err := a.conn.ReadMessage()
		if err != nil {
			log.Printf("[qq] read error: %v", err)
			return err
		}

		var event map[string]interface{}
		if err := json.Unmarshal(data, &event); err != nil {
			continue
		}

		// Only handle incoming message events.
		if event["post_type"] != "message" {
			continue
		}

		a.handleMessage(event)
	}
}

// Stop cancels the context and closes the WebSocket connection.
func (a *Adapter) Stop() error {
	if a.cancel != nil {
		a.cancel()
	}
	if a.conn != nil {
		a.conn.Close()
	}
	return nil
}

// SendText sends a plain-text message via the OneBot send_msg API.
// For group chats, chatID must be prefixed with "group:" (e.g. "group:123456").
// For private chats, chatID is the plain QQ number string.
func (a *Adapter) SendText(chatID, text string) error {
	msgType := "private"
	id := chatID
	if strings.HasPrefix(chatID, "group:") {
		msgType = "group"
		id = strings.TrimPrefix(chatID, "group:")
	}

	payload := map[string]interface{}{
		"action": "send_msg",
		"params": map[string]interface{}{
			"message_type": msgType,
			"user_id":      id,
			"group_id":     id,
			"message":      text,
		},
	}
	data, _ := json.Marshal(payload)
	return a.conn.WriteMessage(websocket.TextMessage, data)
}

// handleMessage processes a OneBot message event and forwards it to the handler.
func (a *Adapter) handleMessage(event map[string]interface{}) {
	msgType, _ := event["message_type"].(string)
	rawMsg, _ := event["raw_message"].(string)
	userID := fmt.Sprintf("%v", event["user_id"])

	// Extract display name from the nested sender object.
	sender, _ := event["sender"].(map[string]interface{})
	userName := ""
	if sender != nil {
		userName, _ = sender["nickname"].(string)
	}

	chatID := userID
	isGroup := msgType == "group"
	if isGroup {
		chatID = fmt.Sprintf("group:%v", event["group_id"])
	}

	msg := channel.InMessage{
		Channel:  "qq",
		ChatID:   chatID,
		UserID:   userID,
		UserName: userName,
		Text:     strings.TrimSpace(rawMsg),
		IsGroup:  isGroup,
	}

	// Detect @bot mentions encoded as CQ codes (e.g. [CQ:at,qq=12345]).
	if strings.Contains(rawMsg, "[CQ:at,qq=") {
		msg.MentionedBot = true
		msg.Text = stripCQCodes(rawMsg)
	}

	if msg.Text == "" {
		return
	}
	if a.handler != nil {
		a.handler(msg)
	}
}

// stripCQCodes removes all CQ code tokens (e.g. "[CQ:at,qq=123]") from s.
func stripCQCodes(s string) string {
	result := s
	for {
		start := strings.Index(result, "[CQ:")
		if start < 0 {
			break
		}
		end := strings.Index(result[start:], "]")
		if end < 0 {
			break
		}
		result = result[:start] + result[start+end+1:]
	}
	return strings.TrimSpace(result)
}
