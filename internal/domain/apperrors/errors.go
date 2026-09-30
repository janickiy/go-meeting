package apperrors

import "errors"

var (
	ErrInvalidInput = errors.New("invalid input")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
)

// Error carries a safe application message, without exposing database errors.
type Error struct {
	Kind    error
	Message string
}

func (e *Error) Error() string             { return e.Message }
func (e *Error) Unwrap() error             { return e.Kind }
func New(kind error, message string) error { return &Error{Kind: kind, Message: message} }
