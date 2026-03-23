package telegram

import (
	"context"
	"fmt"
	"strings"
	"time"

	tele "gopkg.in/telebot.v3"

	"github.com/xavierli/nethelper/internal/channel"
)

// Adapter implements channel.Channel for Telegram via the telebot.v3 SDK.
// It uses long-polling so no public IP or webhook is required.
type Adapter struct {
	token   string
	bot     *tele.Bot
	handler channel.MessageHandler
	ctx     context.Context
	cancel  context.CancelFunc
}

// New creates a new Telegram adapter with the given bot token.
func New(token string) *Adapter {
	return &Adapter{token: token}
}

// Name returns the channel name.
func (a *Adapter) Name() string { return "telegram" }

// Start connects to Telegram via long-polling and begins processing messages.
// It blocks until the context is cancelled.
func (a *Adapter) Start(ctx context.Context, handler channel.MessageHandler) error {
	a.handler = handler
	a.ctx, a.cancel = context.WithCancel(ctx)

	var err error
	a.bot, err = tele.NewBot(tele.Settings{
		Token:  a.token,
		Poller: &tele.LongPoller{Timeout: 10 * time.Second},
	})
	if err != nil {
		return err
	}

	a.bot.Handle(tele.OnText, a.onText)

	// Stop the bot when context is done.
	go func() {
		<-a.ctx.Done()
		a.bot.Stop()
	}()

	a.bot.Start() // blocks until bot.Stop() is called
	return nil
}

// Stop cancels the context, which triggers bot.Stop() in the goroutine above.
func (a *Adapter) Stop() error {
	if a.cancel != nil {
		a.cancel()
	}
	return nil
}

// SendText sends a plain-text message to the given chat ID (integer as string).
// Telegram has a 4096-character limit; long messages are chunked on newlines.
func (a *Adapter) SendText(chatID, text string) error {
	var id int64
	fmt.Sscanf(chatID, "%d", &id)
	recipient := &tele.Chat{ID: id}

	for len(text) > 0 {
		chunk := text
		if len(chunk) > 4000 {
			idx := strings.LastIndex(chunk[:4000], "\n")
			if idx > 0 {
				chunk = chunk[:idx]
			} else {
				chunk = chunk[:4000]
			}
		}
		_, err := a.bot.Send(recipient, chunk)
		if err != nil {
			return err
		}
		text = text[len(chunk):]
	}
	return nil
}

// onText is the telebot handler for plain-text messages.
func (a *Adapter) onText(c tele.Context) error {
	msg := channel.InMessage{
		Channel:  "telegram",
		ChatID:   fmt.Sprintf("%d", c.Chat().ID),
		UserID:   fmt.Sprintf("%d", c.Sender().ID),
		UserName: c.Sender().Username,
		Text:     c.Text(),
		IsGroup:  c.Chat().Type == tele.ChatGroup || c.Chat().Type == tele.ChatSuperGroup,
	}

	// In group chats, check whether the bot's username was @-mentioned.
	if msg.IsGroup {
		botUser := a.bot.Me
		if botUser != nil && strings.Contains(c.Text(), "@"+botUser.Username) {
			msg.MentionedBot = true
			msg.Text = strings.ReplaceAll(msg.Text, "@"+botUser.Username, "")
			msg.Text = strings.TrimSpace(msg.Text)
		}
	}

	if a.handler != nil {
		a.handler(msg)
	}
	return nil
}
