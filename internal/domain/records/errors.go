package records

import "errors"

// ErrConferenceAlreadyRecording означает, что для conferenceId уже действует блокировка запуска записи.
var ErrConferenceAlreadyRecording = errors.New("recording already started for conferenceId")

// ErrRecordStateChanged обозначает устаревшее обновление состояния или нарушение порядка переходов.
var ErrRecordStateChanged = errors.New("record status no longer allows this transition")
