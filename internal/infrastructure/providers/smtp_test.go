package providers

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math/big"
	"mime"
	"mime/multipart"
	"net"
	"net/mail"
	"net/textproto"
	"strconv"
	"strings"
	"testing"
	"time"

	d "github.com/janickiy/go-recorder/internal/domain/integrations"
	"github.com/janickiy/go-recorder/internal/domain/jobs"
)

type smtpTestResult struct {
	username, password, sender, recipient string
	data                                  []byte
	secure                                bool
}

type smtpTestServer struct {
	listener                          net.Listener
	cert                              tls.Certificate
	root                              *x509.CertPool
	results                           chan smtpTestResult
	accepted                          chan struct{}
	mode                              string
	noTLS                             bool
	authCode, recipientCode, quitCode int
}

func newSMTPTestServer(t *testing.T, mode string) *smtpTestServer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "SMTP test"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IsCA: true, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &smtpTestServer{listener: listener, cert: tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, root: x509.NewCertPool(), results: make(chan smtpTestResult, 4), accepted: make(chan struct{}, 4), mode: mode, authCode: 235, recipientCode: 250, quitCode: 221}
	server.root.AddCert(certificate)
	t.Cleanup(func() { _ = listener.Close() })
	return server
}

func (s *smtpTestServer) start() {
	go func() {
		for {
			conn, err := s.listener.Accept()
			if err != nil {
				return
			}
			s.accepted <- struct{}{}
			go s.serve(conn)
		}
	}()
}

func (s *smtpTestServer) serve(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	result := smtpTestResult{}
	defer func() { s.results <- result }()
	if s.mode == "silent" {
		_, _ = io.Copy(io.Discard, conn)
		return
	}
	if s.mode == "tls" {
		secured := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{s.cert}, MinVersion: tls.VersionTLS12})
		if secured.Handshake() != nil {
			return
		}
		conn, result.secure = secured, true
	}
	reader := textproto.NewReader(bufio.NewReader(conn))
	_, _ = fmt.Fprint(conn, "220 smtp.test ESMTP\r\n")
	for {
		line, err := reader.ReadLine()
		if err != nil {
			return
		}
		command, args, _ := strings.Cut(line, " ")
		switch command {
		case "EHLO":
			_, _ = fmt.Fprint(conn, "250-smtp.test\r\n")
			if !result.secure && !s.noTLS {
				_, _ = fmt.Fprint(conn, "250-STARTTLS\r\n")
			}
			_, _ = fmt.Fprint(conn, "250 AUTH PLAIN\r\n")
		case "STARTTLS":
			_, _ = fmt.Fprint(conn, "220 Begin TLS\r\n")
			secured := tls.Server(conn, &tls.Config{Certificates: []tls.Certificate{s.cert}, MinVersion: tls.VersionTLS12})
			if secured.Handshake() != nil {
				return
			}
			conn, result.secure = secured, true
			reader = textproto.NewReader(bufio.NewReader(conn))
		case "AUTH":
			_, encoded, _ := strings.Cut(args, " ")
			decoded, err := base64.StdEncoding.DecodeString(encoded)
			if err == nil {
				parts := strings.Split(string(decoded), "\x00")
				if len(parts) == 3 {
					result.username, result.password = parts[1], parts[2]
				}
			}
			if !result.secure {
				_, _ = fmt.Fprint(conn, "535 AUTH without TLS\r\n")
				return
			}
			_, _ = fmt.Fprintf(conn, "%d server-password-private-response\r\n", s.authCode)
		case "MAIL":
			result.sender = args
			_, _ = fmt.Fprint(conn, "250 Sender accepted\r\n")
		case "RCPT":
			result.recipient = args
			_, _ = fmt.Fprintf(conn, "%d recipient-private-response\r\n", s.recipientCode)
		case "DATA":
			_, _ = fmt.Fprint(conn, "354 Send message\r\n")
			result.data, err = reader.ReadDotBytes()
			if err != nil {
				return
			}
			_, _ = fmt.Fprint(conn, "250 Message accepted\r\n")
		case "QUIT":
			_, _ = fmt.Fprintf(conn, "%d Goodbye\r\n", s.quitCode)
			return
		default:
			_, _ = fmt.Fprint(conn, "500 Unknown command\r\n")
		}
	}
}

func (s *smtpTestServer) adapter(t *testing.T) *SMTPEmail {
	t.Helper()
	_, rawPort, _ := net.SplitHostPort(s.listener.Addr().String())
	port, _ := strconv.Atoi(rawPort)
	mode := s.mode
	if mode == "silent" {
		mode = "starttls"
	}
	adapter, err := NewSMTPEmail(SMTPConfig{Host: "127.0.0.1", Port: port, TLSMode: mode, Username: "test-user", Password: "test-password", From: "Meetrix <sender@example.test>", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	adapter.tlsConfig.RootCAs = s.root
	return adapter
}

func smtpTestMessage() d.EmailMessage {
	return d.EmailMessage{IdempotencyKey: "invitation-stable-id", To: "participant@example.test", Subject: "Приглашение на встречу", Text: "Встреча завтра\nhttps://example.test/i/invite", HTML: "<p>Встреча завтра</p>"}
}

func TestSMTPDeliversAuthenticatedMIMEOverTLS(t *testing.T) {
	for _, mode := range []string{"tls", "starttls"} {
		t.Run(mode, func(t *testing.T) {
			server := newSMTPTestServer(t, mode)
			server.quitCode = 500 // DATA was accepted: failed QUIT must not duplicate a delivered email.
			adapter := server.adapter(t)
			server.start()
			var previousID string
			for range 2 {
				message := smtpTestMessage()
				if err := adapter.Send(context.Background(), message); err != nil {
					t.Fatal(err)
				}
				result := <-server.results
				if !result.secure || result.username != "test-user" || result.password != "test-password" || !strings.HasPrefix(result.sender, "FROM:<sender@example.test>") || result.recipient != "TO:<participant@example.test>" {
					t.Fatal("TLS, AUTH or SMTP envelope missing")
				}
				parsed, err := mail.ReadMessage(strings.NewReader(string(result.data)))
				if err != nil {
					t.Fatal(err)
				}
				subject, err := (&mime.WordDecoder{}).DecodeHeader(parsed.Header.Get("Subject"))
				if err != nil || subject != message.Subject || parsed.Header.Get("From") != "\"Meetrix\" <sender@example.test>" {
					t.Fatal("Unicode subject or sender invalid")
				}
				id := parsed.Header.Get("Message-ID")
				if id == "" || (previousID != "" && id != previousID) || strings.Contains(id, message.IdempotencyKey) {
					t.Fatal("Message-ID must be stable and opaque")
				}
				previousID = id
				mediaType, params, err := mime.ParseMediaType(parsed.Header.Get("Content-Type"))
				if err != nil || mediaType != "multipart/alternative" {
					t.Fatal("multipart/alternative missing")
				}
				body := multipart.NewReader(parsed.Body, params["boundary"])
				for _, expected := range []string{message.Text, message.HTML} {
					part, err := body.NextPart()
					if err != nil {
						t.Fatal(err)
					}
					content, err := io.ReadAll(part)
					if err != nil || strings.ReplaceAll(string(content), "\r\n", "\n") != expected {
						t.Fatal("UTF-8 MIME content invalid")
					}
				}
			}
		})
	}
}

func TestSMTPSanitizesFailuresAndRejectsTLSdowngrade(t *testing.T) {
	for _, tc := range []struct {
		name, code string
		retry      bool
		configure  func(*smtpTestServer, *SMTPEmail)
	}{
		{"auth denied", "smtp_auth", false, func(s *smtpTestServer, _ *SMTPEmail) { s.authCode = 535 }},
		{"temporary recipient failure", "smtp_recipient", true, func(s *smtpTestServer, _ *SMTPEmail) { s.recipientCode = 451 }},
		{"permanent recipient failure", "smtp_recipient", false, func(s *smtpTestServer, _ *SMTPEmail) { s.recipientCode = 550 }},
		{"STARTTLS missing", "smtp_tls_required", false, func(s *smtpTestServer, _ *SMTPEmail) { s.noTLS = true }},
		{"untrusted certificate", "smtp_tls", false, func(_ *smtpTestServer, a *SMTPEmail) { a.tlsConfig.RootCAs = x509.NewCertPool() }},
		{"certificate hostname mismatch", "smtp_tls", false, func(_ *smtpTestServer, a *SMTPEmail) { a.tlsConfig.ServerName = "wrong.example.test" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := newSMTPTestServer(t, "starttls")
			adapter := server.adapter(t)
			tc.configure(server, adapter)
			server.start()
			err := adapter.Send(context.Background(), smtpTestMessage())
			var classified jobs.Error
			if !errors.As(err, &classified) || classified.Code != tc.code || classified.Retryable != tc.retry {
				t.Fatalf("unexpected safe SMTP classification: %v", err)
			}
			if strings.Contains(err.Error(), "password") || strings.Contains(err.Error(), "private-response") || strings.Contains(err.Error(), "participant@") {
				t.Fatal("SMTP error leaked credentials or recipient")
			}
			result := <-server.results
			if (tc.code == "smtp_tls" || tc.code == "smtp_tls_required") && (result.username != "" || len(result.data) != 0) {
				t.Fatal("credentials or content sent before verified TLS")
			}
		})
	}
}

func TestSMTPTimeoutBoundsSilentServer(t *testing.T) {
	server := newSMTPTestServer(t, "silent")
	adapter := server.adapter(t)
	server.start()
	started := time.Now()
	err := adapter.Send(context.Background(), smtpTestMessage())
	var classified jobs.Error
	if !errors.As(err, &classified) || !classified.Retryable || time.Since(started) > 1500*time.Millisecond {
		t.Fatal("SMTP timeout did not bound a silent server")
	}
	<-server.results
}

func TestSMTPCancellationInterruptsGreeting(t *testing.T) {
	server := newSMTPTestServer(t, "silent")
	adapter := server.adapter(t)
	server.start()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- adapter.Send(ctx, smtpTestMessage()) }()
	select {
	case <-server.accepted:
	case err := <-done:
		t.Fatalf("SMTP did not connect: %v", err)
	case <-time.After(time.Second):
		t.Fatal("SMTP did not connect within its timeout")
	}
	cancel()
	select {
	case err := <-done:
		var classified jobs.Error
		if !errors.As(err, &classified) || !classified.Retryable {
			t.Fatal("cancellation must return a safe recoverable error")
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("SMTP ignored context cancellation")
	}
	<-server.results
}

func TestSMTPRejectsHeaderInjectionBeforeConnection(t *testing.T) {
	server := newSMTPTestServer(t, "tls")
	adapter := server.adapter(t)
	for _, change := range []func(*d.EmailMessage){
		func(m *d.EmailMessage) { m.To += "\r\nBcc: stolen@example.test" },
		func(m *d.EmailMessage) { m.Subject += "\r\nBcc: stolen@example.test" },
		func(m *d.EmailMessage) { m.To = "one@example.test, two@example.test" },
		func(m *d.EmailMessage) { m.To = "почта@example.test" },
		func(m *d.EmailMessage) { m.IdempotencyKey = "" },
	} {
		message := smtpTestMessage()
		change(&message)
		if err := adapter.Send(context.Background(), message); err == nil || err.Error() != "smtp_input" {
			t.Fatal("unsafe email accepted")
		}
	}
}

func TestSMTPIntegrationFactory(t *testing.T) {
	server := newSMTPTestServer(t, "tls")
	adapter := server.adapter(t)
	providers, err := NewIntegrations(IntegrationConfig{Email: AdapterConfig{Mode: "smtp"}, SMTP: adapter.config})
	if err != nil || providers.Capabilities.Email != "smtp" {
		t.Fatal("SMTP integration not exposed", err)
	}
	if _, ok := providers.Email.(*SMTPEmail); !ok {
		t.Fatal("SMTP email adapter not selected")
	}
	if _, err := NewIntegrations(IntegrationConfig{Push: AdapterConfig{Mode: "smtp"}}); err == nil {
		t.Fatal("SMTP must only be available for email")
	}
}

func TestSMTPFoldsLongUnicodeHeaders(t *testing.T) {
	adapter, err := NewSMTPEmail(SMTPConfig{Host: "smtp.example.test", Port: 465, TLSMode: "tls", Username: "user", Password: "password", From: strings.Repeat("Организатор ", 12) + "<sender@example.test>"})
	if err != nil {
		t.Fatal(err)
	}
	message := smtpTestMessage()
	message.Subject = strings.Repeat("Встреча ", 100)
	_, raw, err := adapter.message(message)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(raw), "\r\n") {
		if len(line) > 998 {
			t.Fatal("SMTP line exceeds RFC limit")
		}
	}
	parsed, err := mail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := (&mime.WordDecoder{}).DecodeHeader(parsed.Header.Get("Subject"))
	if err != nil || decoded != message.Subject {
		t.Fatal("folded Unicode subject changed")
	}
}
