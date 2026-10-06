package recordings

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
	"github.com/janickiy/go-recorder/internal/domain/records"
)

type dispatchProbe struct {
	steps     []string
	command   records.OutboxCommand
	record    records.Record
	failureAt string
	failure   error
	acquired  bool
	contexts  []context.Context
}

func (p *dispatchProbe) step(ctx context.Context, name string) error {
	p.steps = append(p.steps, name)
	p.contexts = append(p.contexts, ctx)
	if name == p.failureAt {
		return p.failure
	}
	return nil
}
func (p *dispatchProbe) ClaimCommand(ctx context.Context) (records.OutboxCommand, records.Record, error) {
	return p.command, p.record, p.step(ctx, "claim")
}
func (p *dispatchProbe) CompleteCommand(ctx context.Context, _ records.OutboxCommand) error {
	return p.step(ctx, "complete")
}
func (p *dispatchProbe) RetryCommand(ctx context.Context, _ records.OutboxCommand) error {
	return p.step(ctx, "retry")
}
func (p *dispatchProbe) Acquire(ctx context.Context, _, _ string) (bool, error) {
	return p.acquired, p.step(ctx, "acquire")
}
func (p *dispatchProbe) StartRecord(ctx context.Context, _ string, _ int) error {
	return p.step(ctx, "start")
}
func (p *dispatchProbe) StopRecord(ctx context.Context, _, _ string) error {
	return p.step(ctx, "stop")
}
func (p *dispatchProbe) Broadcast(ctx context.Context, event realtime.Envelope) error {
	return p.step(ctx, event.Type)
}

func TestCommandDeliveryOrderingAndFailureOwnership(t *testing.T) {
	cause := errors.New("dependency failed")
	for _, tc := range []struct {
		name, command, status, failureAt string
		lock                             bool
		expected                         []string
		wantErr                          error
	}{
		{"start", "record.start", records.StatusStarting, "", true, []string{"claim", "acquire", "start", "recording.starting", "complete"}, nil},
		{"stop", "record.stop", records.StatusStopping, "", true, []string{"claim", "stop", "recording.stopping", "complete"}, nil},
		{"already active", "record.start", records.StatusRecording, "", true, []string{"claim", "acquire", "start", "complete"}, nil},
		{"claim failed", "record.start", records.StatusStarting, "claim", true, []string{"claim"}, cause},
		{"lock denied", "record.start", records.StatusStarting, "", false, []string{"claim", "acquire", "retry"}, apperrors.ErrConflict},
		{"lock failed", "record.start", records.StatusStarting, "acquire", true, []string{"claim", "acquire", "retry"}, cause},
		{"publish failed", "record.start", records.StatusStarting, "start", true, []string{"claim", "acquire", "start", "retry"}, cause},
		{"stop failed", "record.stop", records.StatusStopping, "stop", true, []string{"claim", "stop", "retry"}, cause},
		{"complete failed", "record.start", records.StatusStarting, "complete", true, []string{"claim", "acquire", "start", "recording.starting", "complete", "retry"}, cause},
		{"event best effort", "record.start", records.StatusStarting, "recording.starting", true, []string{"claim", "acquire", "start", "recording.starting", "complete"}, nil},
		{"terminal", "record.start", records.StatusReady, "", true, []string{"claim", "complete"}, nil},
		{"terminal complete failed", "record.stop", records.StatusReady, "complete", true, []string{"claim", "complete"}, cause},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := &dispatchProbe{command: records.OutboxCommand{CommandType: tc.command}, record: records.Record{Status: tc.status}, failureAt: tc.failureAt, failure: cause, acquired: tc.lock}
			d := NewCommandDispatcher(p, p, p, p)
			begin := time.Now()
			err := d.dispatchOne(context.Background())
			if !errors.Is(err, tc.wantErr) || !reflect.DeepEqual(p.steps, tc.expected) {
				t.Fatalf("steps=%v err=%v expected=%v err=%v", p.steps, err, tc.expected, tc.wantErr)
			}
			for _, ctx := range p.contexts {
				deadline, ok := ctx.Deadline()
				if !ok || deadline.Before(begin) || deadline.After(begin.Add(5*time.Second+100*time.Millisecond)) || ctx.Err() != context.Canceled {
					t.Fatal("operation context not bounded or released")
				}
			}
		})
	}
}

func TestCommandDispatcherShutdownBeforePolling(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := &dispatchProbe{}
	done := make(chan struct{})
	go func() { NewCommandDispatcher(p, p, p, p).Run(ctx); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("dispatcher did not stop")
	}
	if len(p.steps) != 0 {
		t.Fatalf("delivery after cancellation: %v", p.steps)
	}
}
