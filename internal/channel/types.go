package channel

import "context"

type FileRef struct {
	Name string
	URL  string
	Type string // "file" / "image"
}

type InMessage struct {
	Channel      string
	ChatID       string
	UserID       string
	UserName     string
	Text         string
	Files        []FileRef
	IsGroup      bool
	MentionedBot bool
	ReplyTo      string
}

func (m InMessage) UserKey() string {
	return m.Channel + ":" + m.UserID
}

type MessageHandler func(msg InMessage)

type Channel interface {
	Name() string
	Start(ctx context.Context, handler MessageHandler) error
	Stop() error
	SendText(chatID string, text string) error
}

// StreamingChannel extends Channel with card create/update/finalize capability.
// Channels that support this can show real-time progress by sending an initial
// "thinking" card and patching it as the agent calls tools.
type StreamingChannel interface {
	Channel
	// SendInitCard creates a streaming card and returns its ID (card_id, not message_id).
	SendInitCard(chatID string, text string) (cardID string, err error)
	// UpdateCard updates card content while keeping streaming mode active.
	UpdateCard(chatID string, cardID string, text string) error
	// FinalizeCard sends the final update, turns off streaming mode, and marks the card as done.
	FinalizeCard(cardID string, text string) error
}
