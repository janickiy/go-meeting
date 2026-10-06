package captions

import (
	"encoding/binary"
	"testing"
)

func TestAudioActivityPCMThreshold(t *testing.T) {
	for _, tc := range []struct {
		name     string
		samples  []int16
		trailing bool
		active   bool
	}{
		{"empty", nil, false, false}, {"incomplete sample", nil, true, false},
		{"silence", []int16{0, 0, 0}, false, false},
		{"below threshold", []int16{260, -260}, false, false},
		{"above threshold", []int16{261, -261}, false, true},
		{"negative full scale", []int16{-32768}, false, true},
		{"positive full scale", []int16{32767}, false, true},
		{"odd trailing byte ignored", []int16{260, -260}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pcm := make([]byte, len(tc.samples)*2)
			for i, sample := range tc.samples {
				binary.LittleEndian.PutUint16(pcm[2*i:], uint16(sample))
			}
			if tc.trailing {
				pcm = append(pcm, 0xff)
			}
			if got := AudioActive(pcm); got != tc.active {
				t.Fatalf("active=%v want=%v", got, tc.active)
			}
		})
	}
}
