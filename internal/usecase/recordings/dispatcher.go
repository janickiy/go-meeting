package recordings

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/janickiy/meet-space/internal/domain/apperrors"
	"github.com/janickiy/meet-space/internal/domain/records"
)

// CommandRepository leases commands already committed with the recording state.
type CommandRepository interface {
	ClaimCommand(context.Context) (records.OutboxCommand, records.Record, error)
	CompleteCommand(context.Context, records.OutboxCommand) error
	RetryCommand(context.Context, records.OutboxCommand) error
}

type Commander interface {
	StartRecord(context.Context, string, int) error
	StopRecord(context.Context, string, string) error
}

// Locker reserves the recording before delivery. The capture worker owns release.
type Locker interface {
	Acquire(context.Context, string, string) (bool, error)
}

// CommandDispatcher owns outbox polling and delivery ordering, independent of
// HTTP reads/authorization. Its caller owns cancellation and shared clients.
type CommandDispatcher struct {
	repo     CommandRepository
	commands Commander
	locker   Locker
	events   Events
}

func NewCommandDispatcher(repo CommandRepository, commands Commander, locker Locker, events Events) *CommandDispatcher {
	return &CommandDispatcher{repo: repo, commands: commands, locker: locker, events: events}
}

func (d *CommandDispatcher) Run(ctx context.Context) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		for range 16 {
			if err := d.dispatchOne(ctx); err != nil {
				break
			}
		}
	}
}

// dispatchOne keeps the lease, publication, completion and retry in one bounded
// operation. A terminal recording only completes its command: it is not retried
// through delivery, preserving the existing recovery/fencing contract.
func (d *CommandDispatcher) dispatchOne(ctx context.Context) error {
	op, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	command, record, err := d.repo.ClaimCommand(op)
	if err != nil {
		return err
	}
	if records.IsTerminalStatus(record.Status) {
		return d.repo.CompleteCommand(op, command)
	}
	err = d.deliver(op, command, record)
	if err == nil {
		err = d.repo.CompleteCommand(op, command)
	}
	if err != nil {
		_ = d.repo.RetryCommand(op, command)
		if !errors.Is(err, context.Canceled) {
			slog.Warn("recording command retry", "recording_id", record.UUID, "command", command.CommandType)
		}
	}
	return err
}

func (d *CommandDispatcher) deliver(ctx context.Context, command records.OutboxCommand, record records.Record) error {
	if command.CommandType != "record.start" {
		if err := d.commands.StopRecord(ctx, record.UUID, command.Reason); err != nil {
			return err
		}
		publish(d.events, ctx, record, "recording.stopping")
		return nil
	}
	if d.locker != nil {
		acquired, err := d.locker.Acquire(ctx, record.ConferenceID, record.UUID)
		if err != nil {
			return err
		}
		if !acquired {
			return apperrors.ErrConflict
		}
	}
	if err := d.commands.StartRecord(ctx, record.UUID, record.SegmentDurationSec); err != nil {
		return err
	}
	if record.Status == records.StatusStarting {
		publish(d.events, ctx, record, "recording.starting")
	}
	return nil
}
