package records

import "errors"

// ErrConferenceAlreadyRecording означает, что для conferenceId уже есть активный start lock.
var ErrConferenceAlreadyRecording = errors.New("recording already started for conferenceId")

// ErrRecordStateChanged indicates a stale or out-of-order lifecycle update.
var ErrRecordStateChanged = errors.New("record status no longer allows this transition")
