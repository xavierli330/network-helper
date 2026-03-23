package discord

import (
	"context"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/xavierli/nethelper/internal/channel"
)

// Adapter implements channel.Channel for Discord via the discordgo SDK.
type Adapter struct {
	token   string
	session *discordgo.Session
	handler channel.MessageHandler
	botID   string
	ctx     context.Context
	cancel  context.CancelFunc
}

// New creates a new Discord adapter with the given bot token.
func New(token string) *Adapter {
	return &Adapter{token: token}
}

// Name returns the channel name.
func (a *Adapter) Name() string { return "discord" }

// Start connects to Discord via WebSocket and begins processing messages.
// It blocks until the context is cancelled or a fatal error occurs.
func (a *Adapter) Start(ctx context.Context, handler channel.MessageHandler) error {
	a.handler = handler
	a.ctx, a.cancel = context.WithCancel(ctx)

	var err error
	a.session, err = discordgo.New("Bot " + a.token)
	if err != nil {
		return err
	}

	a.session.AddHandler(a.onMessage)
	a.session.Identify.Intents = discordgo.IntentsGuildMessages |
		discordgo.IntentsDirectMessages |
		discordgo.IntentMessageContent

	if err := a.session.Open(); err != nil {
		return err
	}
	a.botID = a.session.State.User.ID

	<-a.ctx.Done()
	return a.session.Close()
}

// Stop cancels the context, causing the WebSocket connection to close.
func (a *Adapter) Stop() error {
	if a.cancel != nil {
		a.cancel()
	}
	return nil
}

// SendText sends a plain-text message to the given channel ID.
// Discord has a 2000-character limit; long messages are chunked on newlines.
func (a *Adapter) SendText(chatID, text string) error {
	for len(text) > 0 {
		chunk := text
		if len(chunk) > 1900 {
			// Prefer splitting on a newline boundary.
			idx := strings.LastIndex(chunk[:1900], "\n")
			if idx > 0 {
				chunk = chunk[:idx]
			} else {
				chunk = chunk[:1900]
			}
		}
		_, err := a.session.ChannelMessageSend(chatID, chunk)
		if err != nil {
			return err
		}
		text = text[len(chunk):]
	}
	return nil
}

// onMessage is the discordgo event handler for MessageCreate events.
func (a *Adapter) onMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == a.botID {
		return // ignore self
	}

	isGroup := m.GuildID != "" // GuildID set = server channel; empty = DM
	mentioned := false
	for _, u := range m.Mentions {
		if u.ID == a.botID {
			mentioned = true
			break
		}
	}

	// Strip @mention tokens from text so the LLM receives clean input.
	text := m.Content
	text = strings.ReplaceAll(text, "<@"+a.botID+">", "")
	text = strings.ReplaceAll(text, "<@!"+a.botID+">", "")
	text = strings.TrimSpace(text)

	if text == "" {
		return
	}

	msg := channel.InMessage{
		Channel:      "discord",
		ChatID:       m.ChannelID,
		UserID:       m.Author.ID,
		UserName:     m.Author.Username,
		Text:         text,
		IsGroup:      isGroup,
		MentionedBot: mentioned,
	}

	// Attach any file/image attachments.
	for _, att := range m.Attachments {
		fileType := "file"
		if strings.HasPrefix(att.ContentType, "image/") {
			fileType = "image"
		}
		msg.Files = append(msg.Files, channel.FileRef{
			Name: att.Filename,
			URL:  att.URL,
			Type: fileType,
		})
	}

	if a.handler != nil {
		a.handler(msg)
	}
}
