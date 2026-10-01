package sfu

import (
	"log/slog"

	"github.com/pion/logging"
)

// Pion's messages can contain complete remote candidates, SDP and credentials.
// This adapter deliberately retains only bounded component/severity metadata:
// even formatting a supplied argument could invoke an untrusted String method.
type safePionLoggerFactory struct {
	logger *slog.Logger
}

type safePionLogger struct {
	logger    *slog.Logger
	component string
}

var _ logging.LoggerFactory = safePionLoggerFactory{}
var _ logging.LeveledLogger = safePionLogger{}

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

func (l safePionLogger) Trace(string)          {}
func (l safePionLogger) Tracef(string, ...any) {}
func (l safePionLogger) Debug(string)          {}
func (l safePionLogger) Debugf(string, ...any) {}

func (l safePionLogger) Info(string) {
	l.logger.Info("media transport event", "event_type", "pion_info", "component", l.component)
}

func (l safePionLogger) Infof(string, ...any) { l.Info("") }

func (l safePionLogger) Warn(string) {
	l.logger.Warn("media transport event", "event_type", "pion_warning", "component", l.component)
}

func (l safePionLogger) Warnf(string, ...any) { l.Warn("") }

func (l safePionLogger) Error(string) {
	l.logger.Error("media transport event", "event_type", "pion_error", "component", l.component)
}

func (l safePionLogger) Errorf(string, ...any) { l.Error("") }
