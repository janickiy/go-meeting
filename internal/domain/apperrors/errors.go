package apperrors

import "errors"

var (
	ErrInvalidInput = errors.New("invalid input")
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")
	ErrNotFound     = errors.New("not found")
	ErrConflict     = errors.New("conflict")
	ErrUnavailable  = errors.New("unavailable")
)

// Error связывает категорию прикладной ошибки с безопасным сообщением для клиента.
//   - Kind: тип события, ошибки или медиа, определяющий ветку обработки.
//   - Message: сообщение чата или безопасный текст ответа согласно указанному типу.
type Error struct {
	Kind    error
	Message string
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
func (e *Error) Unwrap() error { return e.Kind }

// New создаёт прикладную ошибку с категорией и безопасным сообщением.
//
// @args
//   - kind (error): тип события, ошибки или медиа, определяющий ветку обработки.
//   - message (string): сообщение чата или безопасный текст ответа согласно указанному типу.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func New(kind error, message string) error { return &Error{Kind: kind, Message: message} }
