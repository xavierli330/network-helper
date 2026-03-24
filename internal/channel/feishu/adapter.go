package feishu

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync/atomic"
	"time"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
	"github.com/larksuite/oapi-sdk-go/v3/event/dispatcher"
	larkcardkit "github.com/larksuite/oapi-sdk-go/v3/service/cardkit/v1"
	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	larkws "github.com/larksuite/oapi-sdk-go/v3/ws"

	"github.com/xavierli/nethelper/internal/channel"
)

// Adapter implements channel.Channel and channel.StreamingChannel for Feishu (Lark).
type Adapter struct {
	appID     string
	appSecret string
	client    *lark.Client
	wsClient  *larkws.Client
	handler   channel.MessageHandler
	ctx       context.Context
	cancel    context.CancelFunc
}

func New(appID, appSecret string) *Adapter {
	return &Adapter{
		appID:     appID,
		appSecret: appSecret,
		client:    lark.NewClient(appID, appSecret),
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

// ---------- Sending messages ----------

// SendText sends a plain text or card message depending on content.
func (a *Adapter) SendText(chatID, text string) error {
	if containsMarkdown(text) {
		return a.sendCardViaCardKit(chatID, text)
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

// sendCardViaCardKit creates a CardKit card, then sends it as a share_card message.
func (a *Adapter) sendCardViaCardKit(chatID, markdownText string) error {
	cardID, err := a.createCardKitCard(markdownText, false)
	if err != nil {
		// Fallback to plain text
		return a.sendPlainText(chatID, markdownText)
	}
	return a.sendCardByID(chatID, cardID)
}

// ---------- StreamingChannel interface ----------

// SendInitCard creates a streaming CardKit card with "thinking" text, sends it, and returns the card_id.
func (a *Adapter) SendInitCard(chatID, text string) (string, error) {
	// Create a streaming-mode card
	cardID, err := a.createCardKitCard(text, true)
	if err != nil {
		return "", fmt.Errorf("create cardkit card: %w", err)
	}

	// Send card to chat
	if err := a.sendCardByID(chatID, cardID); err != nil {
		return "", fmt.Errorf("send card: %w", err)
	}

	return cardID, nil
}

// UpdateCard updates an existing CardKit card with new Markdown content.
// cardID is the card_id returned by SendInitCard (NOT message_id).
func (a *Adapter) UpdateCard(chatID, cardID, text string) error {
	return a.updateCardKitCard(cardID, text, true)
}

// FinalizeCard sends the final update and turns off streaming mode.
func (a *Adapter) FinalizeCard(cardID, text string) error {
	return a.updateCardKitCard(cardID, text, false)
}

// ---------- CardKit operations ----------

var cardSeq int64 // global sequence counter for batch updates

// createCardKitCard creates a card via CardKit API and returns card_id.
func (a *Adapter) createCardKitCard(markdownText string, streaming bool) (string, error) {
	cardData := map[string]interface{}{
		"config": map[string]interface{}{
			"streaming_mode": streaming,
		},
		"header": map[string]interface{}{
			"title": map[string]interface{}{
				"tag":     "plain_text",
				"content": "nethelper",
			},
			"template": "blue",
		},
		"body": map[string]interface{}{
			"tag": "div",
			"elements": []interface{}{
				map[string]interface{}{
					"tag":        "markdown",
					"content":    markdownText,
					"element_id": "md_content",
				},
			},
		},
	}

	dataJSON, _ := json.Marshal(cardData)

	req := larkcardkit.NewCreateCardReqBuilder().
		Body(larkcardkit.NewCreateCardReqBodyBuilder().
			Type("card_json").
			Data(string(dataJSON)).
			Build()).
		Build()

	resp, err := a.client.Cardkit.V1.Card.Create(a.ctx, req)
	if err != nil {
		return "", err
	}
	if !resp.Success() {
		return "", fmt.Errorf("cardkit create failed: code=%d msg=%s", resp.Code, resp.Msg)
	}
	if resp.Data == nil || resp.Data.CardId == nil {
		return "", fmt.Errorf("cardkit create: no card_id")
	}
	return *resp.Data.CardId, nil
}

// sendCardByID sends a card message referencing an existing card_id.
func (a *Adapter) sendCardByID(chatID, cardID string) error {
	content, _ := json.Marshal(map[string]string{"type": "card_id", "data": cardID})

	req := larkim.NewCreateMessageReqBuilder().
		ReceiveIdType(larkim.ReceiveIdTypeChatId).
		Body(larkim.NewCreateMessageReqBodyBuilder().
			ReceiveId(chatID).
			MsgType(larkim.MsgTypeInteractive).
			Content(string(content)).
			Build()).
		Build()
	_, err := a.client.Im.Message.Create(a.ctx, req)
	return err
}

// updateCardKitCard updates card content via BatchUpdate.
func (a *Adapter) updateCardKitCard(cardID, markdownText string, streaming bool) error {
	seq := atomic.AddInt64(&cardSeq, 1)

	// Build actions: update the markdown element + optionally toggle streaming
	actions := []map[string]interface{}{
		{
			"action": "update_element",
			"params": map[string]interface{}{
				"element_id": "md_content",
				"element": map[string]interface{}{
					"tag":        "markdown",
					"content":    markdownText,
					"element_id": "md_content",
				},
			},
		},
	}

	// If finalizing (streaming=false), add settings update to turn off streaming + change header
	if !streaming {
		actions = append(actions, map[string]interface{}{
			"action": "partial_update_setting",
			"params": map[string]interface{}{
				"settings": map[string]interface{}{
					"config": map[string]interface{}{
						"streaming_mode": false,
					},
					"header": map[string]interface{}{
						"template": "green",
						"title": map[string]interface{}{
							"tag":     "plain_text",
							"content": "nethelper ✅",
						},
					},
				},
			},
		})
	}

	actionsJSON, _ := json.Marshal(actions)

	req := larkcardkit.NewBatchUpdateCardReqBuilder().
		CardId(cardID).
		Body(larkcardkit.NewBatchUpdateCardReqBodyBuilder().
			Sequence(int(seq)).
			Actions(string(actionsJSON)).
			Uuid(fmt.Sprintf("%s-%d", cardID, seq)).
			Build()).
		Build()

	resp, err := a.client.Cardkit.V1.Card.BatchUpdate(a.ctx, req)
	if err != nil {
		return err
	}
	if !resp.Success() {
		return fmt.Errorf("cardkit batch_update failed: code=%d msg=%s", resp.Code, resp.Msg)
	}
	return nil
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

// Ensure Adapter is unused import safe
var _ = time.Now
