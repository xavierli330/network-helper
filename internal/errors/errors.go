package errors

import "errors"

var (
	ErrDeviceNotFound      = errors.New("device not found")
	ErrInterfaceNotFound   = errors.New("interface not found")
	ErrConfigInvalid       = errors.New("invalid configuration")
	ErrLLMUnavailable      = errors.New("LLM provider unavailable")
	ErrDatabaseNotOpen     = errors.New("database not open")
	ErrPermissionDenied    = errors.New("permission denied")
	ErrInvalidArgument     = errors.New("invalid argument")
	ErrNotImplemented      = errors.New("not implemented")
	ErrTimeout             = errors.New("operation timed out")
	ErrContextCancelled    = errors.New("context cancelled")
	ErrFileNotFound        = errors.New("file not found")
	ErrParseFailed         = errors.New("parse failed")
	ErrToolNotFound        = errors.New("tool not found")
	ErrSessionNotFound     = errors.New("session not found")
	ErrKnowledgeSourceFail = errors.New("knowledge source search failed")
)
