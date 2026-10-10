package sfu

import (
	"github.com/janickiy/meet-space/internal/domain/media"
	"github.com/pion/sdp/v3"
)

// receiveVideoPreference controls only this peer's incoming video. Keeping the
// negotiated senders intact allows resuming camera/screen reception without
// stopping local publications or audio and without allocating new receivers.
func receiveVideoPreference(raw string) (bool, error) {
	var description sdp.SessionDescription
	if err := description.UnmarshalString(raw); err != nil {
		return false, media.ErrInvalid
	}
	enabled, seen := true, false
	for _, attr := range description.Attributes {
		if attr.Key != "x-meet-receive-video" {
			continue
		}
		if seen || (attr.Value != "0" && attr.Value != "1") {
			return false, media.ErrInvalid
		}
		seen = true
		enabled = attr.Value == "1"
	}
	return enabled, nil
}
