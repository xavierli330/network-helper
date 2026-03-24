package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"

	"github.com/xavierli/nethelper/internal/channel"
)

// Adapter implements channel.Channel and channel.StreamingChannel for Feishu.
type Adapter struct {
	appID     string
	appSecret string
	client    *lark.Client
	wsClient  *larkws.Client
	handler   channel.MessageHandler
	ctx       context.Context
	cancel    context.CancelFunc

	// dedup: prevent processing the same message twice (Feishu retries on slow handlers)
	seenMu  sync.Mutex
	seenIDs map[string]bool
}

func New(appID, appSecret string) *Adapter {
	return &Adapter{
		appID:     appID,
		appSecret: appSecret,
		client:    lark.NewClient(appID, appSecret),
		seenIDs:   make(map[string]bool),
	}
}

func (a *Adapter) Name() string { return "feishu" }

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

func (a *Adapter) Stop() error {
	if a.cancel != nil {
		a.cancel()
	}
	return nil
}

// ---------- Sending ----------

func (a *Adapter) SendText(chatID, text string) error {
	if containsMarkdown(text) {
		return a.sendCard(chatID, text, "green")
	}
	return a.sendPlainText(chatID, text)
}

func (a *Adapter) sendPlainText(chatID, text string) error {
	content, _ := json.Marshal(map[string]string{"text": text})
	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(larkim.ReceiveIdTypeChatId).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(chatID).
			MsgType(larkim.MsgTypeText).
			Content(string(content)).
			Build()).
		Build()
	_, err := a.client.Im.Message.Create(a.ctx, req)
	return err
}

// sendCard sends an inline interactive card and returns the message_id.
func (a *Adapter) sendCard(chatID, markdownText, headerColor string) error {
	_, err := a.sendCardGetID(chatID, markdownText, headerColor)
	return err
}

func (a *Adapter) sendCardGetID(chatID, markdownText, headerColor string) (string, error) {
	card := buildCardJSON(markdownText, headerColor)
	content, _ := json.Marshal(card)

	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(larkim.ReceiveIdTypeChatId).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(chatID).
			MsgType(larkim.MsgTypeInteractive).
			Content(string(content)).
			Build()).
		Build()

	resp, err := a.client.Im.Message.Create(a.ctx, req)
	if err != nil {
		return "", err
	}
	if resp != nil && resp.Data != nil && resp.Data.MessageId != nil {
		return *resp.Data.MessageId, nil
	}
	return "", fmt.Errorf("no message_id in response")
}

// ---------- StreamingChannel interface ----------

// SendInitCard sends a "thinking" card and returns the message_id.
func (a *Adapter) SendInitCard(chatID, text string) (string, error) {
	return a.sendCardGetID(chatID, text, "blue")
}

// UpdateCard patches an existing card message with new content.
func (a *Adapter) UpdateCard(chatID, messageID, text string) error {
	card := buildCardJSON(text, "blue")
	content, _ := json.Marshal(card)

	req := larkim.NewPatchMessageReqBuilder().
		MessageId(messageID).
		Body(larkim.NewPatchMessageReqBodyBuilder().
			Content(string(content)).
			Build()).
		Build()

	_, err := a.client.Im.Message.Patch(a.ctx, req)
	return err
}

// FinalizeCard patches the card with final content and green header.
func (a *Adapter) FinalizeCard(messageID, text string) error {
	card := buildCardJSON(text, "green")
	content, _ := json.Marshal(card)

	req := larkim.NewPatchMessageReqBuilder().
		MessageId(messageID).
		Body(larkim.NewPatchMessageReqBodyBuilder().
			Content(string(content)).
			Build()).
		Build()

	_, err := a.client.Im.Message.Patch(a.ctx, req)
	return err
}

// ---------- Card JSON builder ----------

func buildCardJSON(markdownText, headerColor string) map[string]interface{} {
	headerTitle := "nethelper"
	if headerColor == "blue" {
		headerTitle = "nethelper ⏳"
	} else if headerColor == "green" {
		headerTitle = "nethelper ✅"
	}

	return map[string]interface{}{
		"config": map[string]interface{}{
			"wide_screen_mode": true,
		},
		"header": map[string]interface{}{
			"title": map[string]interface{}{
				"tag":     "plain_text",
				"content": headerTitle,
			},
			"template": headerColor,
		},
		"elements": []interface{}{
			map[string]interface{}{
				"tag":     "markdown",
				"content": markdownText,
			},
		},
	}
}

// ---------- Markdown detection ----------

func containsMarkdown(text string) bool {
	indicators := []string{"# ", "**", "```", "| ", "- [", "- ✅", "- ⚠️", "## "}
	for _, ind := range indicators {
		if strings.Contains(text, ind) {
			return true
		}
	}
	return false
}

// ---------- Message reception ----------

func (a *Adapter) onMessage(ctx context.Context, event *larkim.P2MessageReceiveV1) error {
	if event == nil || event.Event == nil {
		return nil
	}

	ev := event.Event
	msg := ev.Message
	if msg == nil {
		return nil
	}

	// Dedup: Feishu may redeliver events if handler is slow.
	// Use message_id to detect duplicates.
	msgID := derefStr(msg.MessageId)
	if msgID != "" {
		a.seenMu.Lock()
		if a.seenIDs[msgID] {
			a.seenMu.Unlock()
			log.Printf("[feishu] skipping duplicate message: %s", msgID)
			return nil
		}
		a.seenIDs[msgID] = true
		// Keep map bounded — clear old entries if too large
		if len(a.seenIDs) > 1000 {
			a.seenIDs = map[string]bool{msgID: true}
		}
		a.seenMu.Unlock()
	}

	msgType := derefStr(msg.MessageType)
	if msgType != larkim.MsgTypeText {
		log.Printf("[feishu] ignoring non-text message type: %s", msgType)
		return nil
	}

	rawContent := derefStr(msg.Content)
	text := extractText(rawContent)
	chatID := derefStr(msg.ChatId)
	chatType := derefStr(msg.ChatType)
	isGroup := chatType == "group" || chatType == "topic_group"

	var userID string
	if ev.Sender != nil && ev.Sender.SenderId != nil {
		userID = derefStr(ev.Sender.SenderId.OpenId)
	}

	mentionedBot := false
	if isGroup {
		for _, m := range msg.Mentions {
			if m == nil {
				continue
			}
			key := derefStr(m.Key)
			if strings.HasPrefix(key, "@_") {
				mentionedBot = true
				break
			}
		}
	} else {
		mentionedBot = true
	}

	cleanText := stripMentions(text)
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

func extractText(raw string) string {
	var m map[string]string
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return raw
	}
	return m["text"]
}

func stripMentions(text string) string {
	parts := strings.Fields(text)
	var clean []string
	for _, p := range parts {
		if strings.HasPrefix(p, "@") {
			continue
		}
		clean = append(clean, p)
	}
	return strings.TrimSpace(strings.Join(clean, " "))
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
