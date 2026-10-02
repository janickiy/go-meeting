package sfu

import (
	"log/slog"

	"github.com/pion/logging"
)

// safePionLoggerFactory создаёт безопасный адаптер логов Pion, сохраняющий только ограниченные сведения компонента и уровня.
//   - logger: значение logger типа *slog.Logger, используемое согласно назначению этой операции.
type safePionLoggerFactory struct {
	logger *slog.Logger
}

// safePionLogger отбрасывает потенциально чувствительные SDP, ICE и аргументы сообщений Pion.
//   - logger: значение logger типа *slog.Logger, используемое согласно назначению этой операции.
//   - component: значение component типа string, используемое согласно назначению этой операции.
type safePionLogger struct {
	logger    *slog.Logger
	component string
}

var _ logging.LoggerFactory = safePionLoggerFactory{}
var _ logging.LeveledLogger = safePionLogger{}

// NewLogger создаёт и связывает зависимости компонента Logger, используемого в пересылке WebRTC-медиа через SFU.
//
// @args
//   - scope (string): область собственных и участвующих встреч пользователя.
//
// @return:
//   - результат 1 (logging.LeveledLogger): созданный компонент с переданными зависимостями.
func (f safePionLoggerFactory) NewLogger(scope string) logging.LeveledLogger {
	component := "other"
	switch scope {
	case "api", "pc", "ice", "dtls", "srtp", "sctp", "ortc", "mux", "datachannel",
		"nack_generator", "nack_responder", "receiver_interceptor", "sender_interceptor",
		"stats", "twcc_sender_interceptor":
		component = scope
	case "RTPReceiver":
		component = "rtp_receiver"
	case "RTPSender":
		component = "rtp_sender"
	case "DTLSTransport":
		component = "dtls_transport"
	}
	return safePionLogger{logger: f.logger, component: component}
}

// Trace обрабатывает сообщение соответствующего уровня через безопасный адаптер, не раскрывая текст и чувствительные аргументы Pion.
//
// @args
//   - аргумент 1 (string): значение для проверки, нормализации или преобразования.
func (l safePionLogger) Trace(string) {}

// Tracef обрабатывает сообщение соответствующего уровня через безопасный адаптер, не раскрывая текст и чувствительные аргументы Pion.
//
// @args
//   - аргумент 1 (string): значение для проверки, нормализации или преобразования.
//   - аргумент 2 (...any): значение для проверки, нормализации или преобразования.
func (l safePionLogger) Tracef(string, ...any) {}

// Debug обрабатывает сообщение соответствующего уровня через безопасный адаптер, не раскрывая текст и чувствительные аргументы Pion.
//
// @args
//   - аргумент 1 (string): значение для проверки, нормализации или преобразования.
func (l safePionLogger) Debug(string) {}

// Debugf обрабатывает сообщение соответствующего уровня через безопасный адаптер, не раскрывая текст и чувствительные аргументы Pion.
//
// @args
//   - аргумент 1 (string): значение для проверки, нормализации или преобразования.
//   - аргумент 2 (...any): значение для проверки, нормализации или преобразования.
func (l safePionLogger) Debugf(string, ...any) {}

// Info обрабатывает сообщение соответствующего уровня через безопасный адаптер, не раскрывая текст и чувствительные аргументы Pion.
//
// @args
//   - аргумент 1 (string): значение для проверки, нормализации или преобразования.
func (l safePionLogger) Info(string) {
	l.logger.Info("media transport event", "event_type", "pion_info", "component", l.component)
}

// Infof обрабатывает сообщение соответствующего уровня через безопасный адаптер, не раскрывая текст и чувствительные аргументы Pion.
//
// @args
//   - аргумент 1 (string): значение для проверки, нормализации или преобразования.
//   - аргумент 2 (...any): значение для проверки, нормализации или преобразования.
func (l safePionLogger) Infof(string, ...any) { l.Info("") }

// Warn обрабатывает сообщение соответствующего уровня через безопасный адаптер, не раскрывая текст и чувствительные аргументы Pion.
//
// @args
//   - аргумент 1 (string): значение для проверки, нормализации или преобразования.
func (l safePionLogger) Warn(string) {
	l.logger.Warn("media transport event", "event_type", "pion_warning", "component", l.component)
}

// Warnf обрабатывает сообщение соответствующего уровня через безопасный адаптер, не раскрывая текст и чувствительные аргументы Pion.
//
// @args
//   - аргумент 1 (string): значение для проверки, нормализации или преобразования.
//   - аргумент 2 (...any): значение для проверки, нормализации или преобразования.
func (l safePionLogger) Warnf(string, ...any) { l.Warn("") }

// Error обрабатывает сообщение уровня ошибки через безопасный адаптер, не раскрывая текст и чувствительные аргументы Pion.
//
// @args
//   - аргумент 1 (string): значение для проверки, нормализации или преобразования.
func (l safePionLogger) Error(string) {
	l.logger.Error("media transport event", "event_type", "pion_error", "component", l.component)
}

// Errorf обрабатывает сообщение соответствующего уровня через безопасный адаптер, не раскрывая текст и чувствительные аргументы Pion.
//
// @args
//   - аргумент 1 (string): значение для проверки, нормализации или преобразования.
//   - аргумент 2 (...any): значение для проверки, нормализации или преобразования.
func (l safePionLogger) Errorf(string, ...any) { l.Error("") }
