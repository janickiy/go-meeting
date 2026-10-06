package apperrors

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
)

func TestSafeClassificationRetainsCause(t *testing.T) {
	cause := &net.OpError{Op: "dial", Net: "tcp", Err: context.DeadlineExceeded}
	err := fmt.Errorf("operation: %w", Wrap(ErrUnavailable, cause, "service unavailable"))
	var operation *net.OpError
	if !errors.Is(err, ErrUnavailable) || !errors.Is(err, context.DeadlineExceeded) || !errors.As(err, &operation) || operation != cause {
		t.Fatalf("category or root cause lost: %v", err)
	}
	if Classify(err) != Unavailable || Classify(cause) != Internal {
		t.Fatal("implicit deadline promotion")
	}
	if got := Wrap(ErrInternal, ErrInvalidInput, "private"); Classify(got) != Internal {
		t.Fatal("cause overrides explicit classification")
	}
	if Wrap(ErrUnavailable, cause, "service unavailable").Error() != "service unavailable" {
		t.Fatal("diagnostic cause exposed")
	}
}
