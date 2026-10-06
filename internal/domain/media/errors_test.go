package media

import (
	"errors"
	"fmt"
	"testing"
)

func TestProtocolErrorsRoundTrip(t *testing.T) {
	cases := []struct {
		code string
		err  error
	}{
		{"media_unavailable", ErrUnavailable}, {"invalid_media_signal", ErrInvalid},
		{"media_unauthorized", ErrUnauthorized}, {"media_ownership_lost", ErrOwnership},
		{"media_limit_exceeded", ErrLimit}, {"media_peer_not_found", ErrPeerNotFound},
		{"media_negotiation_conflict", ErrNegotiation}, {"screen_sharing_conflict", ErrScreenConflict},
		{"media_policy_blocked", ErrPolicy},
	}
	for _, test := range cases {
		if code := ErrorCode(fmt.Errorf("diagnostics: %w", test.err)); code != test.code {
			t.Fatalf("code=%s want=%s", code, test.code)
		}
		if err := ErrorFromCode(test.code); !errors.Is(err, test.err) {
			t.Fatalf("decoded=%v want=%v", err, test.err)
		}
	}
	for _, text := range []string{"", "SQL: private password", "wrapped: media_unauthorized"} {
		if ErrorFromCode(text) != ErrUnavailable || ErrorCode(errors.New(text)) != "media_unavailable" {
			t.Fatal("untrusted human error was interpreted as protocol code")
		}
	}
}
