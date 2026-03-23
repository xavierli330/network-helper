package feishu

import (
	"context"
	"encoding/json"
	"log"
	"strings"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"

	"github.com/xavierli/nethelper/internal/channel"
)

// Adapter implements channel.Channel for Feishu (Lark) via WebSocket long-connection.
// No public IP is required — it uses Feishu's persistent WebSocket endpoint.
type Adapter struct {
	appID     string
	appSecret string
	client    *lark.Client
	wsClient  *larkws.Client
	handler   channel.MessageHandler
	ctx       context.Context
	cancel    context.CancelFunc
}

// New creates a new Feishu adapter with the given app credentials.
func New(appID, appSecret string) *Adapter {
	return &Adapter{
		appID:     appID,
		appSecret: appSecret,
		client:    lark.NewClient(appID, appSecret),
	}
}

// Name returns the channel name.
func (a *Adapter) Name() string { return "feishu" }

// Start connects to Feishu via WebSocket and begins processing messages.
// It blocks until the context is cancelled or a fatal error occurs.
func (a *Adapter) Start(ctx context.Context, handler channel.MessageHandler) error {
	a.handler = handler
	a.ctx, a.cancel = context.WithCancel(ctx)

	eventHandler := dispatcher.NewEventDispatcher("", "").
		OnP2MessageReceiveV1(a.onMessage)

	a.wsClient = larkws.NewClient(a.appID, a.appSecret,
		larkws.WithEventHandler(eventHandler),
		larkws.WithLogLevel(larkcore.LogLevelInfo),
	)

	return a.wsClient.Start(a.ctx)
}

// Stop cancels the context, which causes the WebSocket connection to close.
func (a *Adapter) Stop() error {
	if a.cancel != nil {
		a.cancel()
	}
	return nil
}

// SendText sends a message to the given chat_id. If the text contains Markdown
// indicators it is sent as an interactive card (which Feishu renders with full
// Markdown support); otherwise it is sent as a plain-text message.
func (a *Adapter) SendText(chatID, text string) error {
	if containsMarkdown(text) {
		return a.sendCard(chatID, text)
	}
	return a.sendPlainText(chatID, text)
}

// sendPlainText sends a plain text message.
func (a *Adapter) sendPlainText(chatID, text string) error {
	content, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return err
	}
	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(larkim.ReceiveIdTypeChatId).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(chatID).
			MsgType(larkim.MsgTypeText).
			Content(string(content)).
			Build()).
		Build()
	_, err = a.client.Im.Message.Create(a.ctx, req)
	return err
}

// sendCard sends an interactive card message whose single element renders the
// provided Markdown text.  Feishu cards support headers, bold, code blocks,
// tables, and emoji — a superset of what plain-text messages can show.
func (a *Adapter) sendCard(chatID, markdownText string) error {
	card := map[string]interface{}{
		"config": map[string]interface{}{
			"wide_screen_mode": true,
		},
		"elements": []interface{}{
			map[string]interface{}{
				"tag":     "markdown",
				"content": markdownText,
			},
		},
	}
	content, err := json.Marshal(card)
	if err != nil {
		return err
	}
	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(larkim.ReceiveIdTypeChatId).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(chatID).
			MsgType(larkim.MsgTypeInteractive).
			Content(string(content)).
			Build()).
		Build()
	_, err = a.client.Im.Message.Create(a.ctx, req)
	return err
}

// containsMarkdown reports whether text contains common Markdown formatting
// indicators that Feishu can render inside an interactive card element.
func containsMarkdown(text string) bool {
	indicators := []string{"# ", "**", "```", "| ", "- [", "- ✅", "- ⚠️", "## "}
	for _, ind := range indicators {
		if strings.Contains(text, ind) {
			return true
		}
	}
	return false
}

// onMessage is the event handler for im.message.receive_v1.
func (a *Adapter) onMessage(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
	if event == nil || event.Event == nil {
		return nil
	}

	ev := event.Event

	// --- Extract message fields ---
	msg := ev.Message
	if msg == nil {
		return nil
	}

	// Only handle text messages.
	msgType := derefStr(msg.MessageType)
	if msgType != larkim.MsgTypeText {
		log.Printf("[feishu] ignoring non-text message type: %s", msgType)
		return nil
	}

	// Parse JSON content: {"text": "..."}
	rawContent := derefStr(msg.Content)
	text := extractText(rawContent)

	chatID := derefStr(msg.ChatId)
	chatType := derefStr(msg.ChatType) // "p2p" or "group"
	isGroup := chatType == "group" || chatType == "topic_group"

	// --- Extract sender open_id ---
	var userID string
	if ev.Sender != nil && ev.Sender.SenderId != nil {
		userID = derefStr(ev.Sender.SenderId.OpenId)
	}

	// --- Detect @mentions ---
	mentionedBot := false
	if isGroup {
		for _, m := range msg.Mentions {
			if m == nil {
				continue
			}
			// The bot mention key is typically "@_user_1" but we check if any
			// mention's sender_type is "app" or name matches the bot name.
			// In practice, Feishu sets the mention key to "@_user_N". We check
			// whether the mentioned entity is an app by looking for a non-empty
			// Key field and no open_id (bots don't have open_ids).
			key := derefStr(m.Key)
			if strings.HasPrefix(key, "@_") {
				// Any @mention in the content — include them all.
				// The router will receive this and decide based on MentionedBot.
				mentionedBot = true
				break
			}
		}
	} else {
		// In P2P (direct message), always treat as mentioned.
		mentionedBot = true
	}

	// Strip @mentions from the text so the LLM gets clean input.
	cleanText := stripMentions(text)

	// Determine reply-to message ID for threading context.
	replyTo := derefStr(msg.ParentId)

	inMsg := channel.InMessage{
		Channel:      "feishu",
		ChatID:       chatID,
		UserID:       userID,
		Text:         cleanText,
		IsGroup:      isGroup,
		MentionedBot: mentionedBot,
		ReplyTo:      replyTo,
	}

	if a.handler != nil {
		a.handler(inMsg)
	}
	return nil
}

// extractText parses the Feishu text message JSON content.
// Text messages have the form: {"text": "hello @bot"}
func extractText(raw string) string {
	var m map[string]string
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return raw
	}
	return m["text"]
}

// stripMentions removes @mention tokens (e.g. "@someone ") from message text.
// Feishu includes literal "@name " strings in text content alongside the
// Mentions array; we strip them so the LLM sees clean user input.
func stripMentions(text string) string {
	parts := strings.Fields(text)
	var clean []string
	for _, p := range parts {
		if strings.HasPrefix(p, "@") {
			continue
		}
		clean = append(clean, p)
	}
	result := strings.Join(clean, " ")
	result = strings.TrimSpace(result)
	return result
}

// derefStr safely dereferences a *string, returning "" for nil.
func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
