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

// StreamingChannel extends Channel with message update capability.
// Channels that support this can show real-time progress by sending an initial
// "thinking" card and patching it as the agent calls tools.
type StreamingChannel interface {
	Channel
	// SendInitCard sends an initial "thinking" card and returns its message ID.
	SendInitCard(chatID string, text string) (messageID string, err error)
	// UpdateCard updates an existing card message with new content.
	UpdateCard(chatID string, messageID string, text string) error
}
