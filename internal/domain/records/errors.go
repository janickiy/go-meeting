package records

import "errors"

// ErrConferenceAlreadyRecording означает, что для conferenceId уже есть активный start lock.
var ErrConferenceAlreadyRecording = errors.New("recording already started for conferenceId")
