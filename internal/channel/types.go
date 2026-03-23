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
