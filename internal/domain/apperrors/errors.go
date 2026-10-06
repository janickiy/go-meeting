package apperrors

import "errors"

var (
	ErrInvalidInput = errors.New("invalid input")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
	ErrUnavailable  = errors.New("unavailable")
	ErrRateLimited  = errors.New("rate limited")
	ErrTimeout      = errors.New("timeout")
	ErrInternal     = errors.New("internal")
)

// Error связывает категорию прикладной ошибки с безопасным сообщением для клиента.
//   - Kind: тип события, ошибки или медиа, определяющий ветку обработки.
//   - Message: сообщение чата или безопасный текст ответа согласно указанному типу.
type Error struct {
	Kind    error
	Message string
	Cause   error
}

// Error возвращает текст ошибки без раскрытия внутренних диагностических данных.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
func (e *Error) Error() string { return e.Message }

// Unwrap возвращает причину ошибки для стандартных errors.Is и errors.As.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (e *Error) Unwrap() error {
	if e.Cause != nil {
		return e.Cause
	}
	return e.Kind
}

// Is preserves the application category independently of the diagnostic cause.
func (e *Error) Is(target error) bool { return errors.Is(e.Kind, target) }

// Wrap retains a diagnostic cause without exposing it through Error().
func Wrap(kind, cause error, message string) error {
	return &Error{Kind: kind, Message: message, Cause: cause}
}

// New создаёт прикладную ошибку с категорией и безопасным сообщением.
//
// @args
//   - kind (error): тип события, ошибки или медиа, определяющий ветку обработки.
//   - message (string): сообщение чата или безопасный текст ответа согласно указанному типу.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func New(kind error, message string) error { return &Error{Kind: kind, Message: message} }

// Category is independent of transport status codes and human error messages.
type Category uint8

const (
	Internal Category = iota
	Validation
	Unauthenticated
	Forbidden
	NotFound
	Conflict
	RateLimited
	Unavailable
	Timeout
)

// Classify only promotes explicit application errors. A raw deadline/DB failure
// stays Internal until its owning operation deliberately classifies it.
func Classify(err error) Category {
	var applicationError *Error
	if errors.As(err, &applicationError) {
		err = applicationError.Kind
	}
	switch {
	case errors.Is(err, ErrInvalidInput):
		return Validation
	case errors.Is(err, ErrUnauthorized):
		return Unauthenticated
	case errors.Is(err, ErrForbidden):
		return Forbidden
	case errors.Is(err, ErrNotFound):
		return NotFound
	case errors.Is(err, ErrConflict):
		return Conflict
	case errors.Is(err, ErrRateLimited):
		return RateLimited
	case errors.Is(err, ErrUnavailable):
		return Unavailable
	case errors.Is(err, ErrTimeout):
		return Timeout
	default:
		return Internal
	}
}
