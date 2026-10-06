package composite

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/janickiy/go-recorder/internal/domain/media"
	"github.com/pion/rtp"
)

// The reference executes the original Flush with its timestamp-age predicate.
// Compare complete encoded output and shared source state through quiet chunks,
// missing/reordered RTP and wrapping sequence numbers and timestamps.
func TestFinalFlushPreservesRTPAndQuietSourceReuse(t *testing.T) {
	key := []byte{0, 0, 0, 0x9d, 1, 0x2a, 0x80, 2, 0xe0, 1, 1, 2, 3}
	vp8 := func(sequence uint16, timestamp uint32, marker bool, head bool, payload []byte) *rtp.Packet {
		header := byte(0)
		if head {
			header = 0x10
		}
		return &rtp.Packet{Header: rtp.Header{Version: 2, SequenceNumber: sequence, Timestamp: timestamp, Marker: marker}, Payload: append([]byte{header}, payload...)}
	}
	full := func(sequence uint16, timestamp uint32) *rtp.Packet { return vp8(sequence, timestamp, true, true, key) }
	traces := map[string][]*rtp.Packet{
		"ordered":                {full(10, 90000), full(11, 93600), full(12, 97200)},
		"reordered":              {vp8(10, 90000, false, true, key[:7]), vp8(12, 93600, true, true, key), vp8(11, 90000, true, false, key[7:]), full(13, 97200)},
		"missing_fragment":       {vp8(10, 90000, false, true, key[:7]), vp8(12, 90000, true, false, key[7:]), full(13, 93600), full(14, 97200)},
		"incomplete_tail":        {full(10, 90000), vp8(11, 93600, false, true, key[:7])},
		"missing_partition_head": {vp8(697, 994064077, true, false, key[7:])},
		"sequence_wrap":          {full(65534, 90000), full(65535, 93600), full(0, 97200), full(1, 100800)},
		"timestamp_wrap":         {full(10, ^uint32(0)-7199), full(11, ^uint32(0)-3599), full(12, 0), full(13, 3600)},
		"malformed":              {full(10, 90000), vp8(11, 93600, true, true, []byte{}), full(12, 97200)},
		"empty":                  {},
	}
	for name, packets := range traces {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			track := media.EgressTrack{Track: media.Track{ID: "camera", Kind: media.KindVideo, Source: media.SourceCamera}, MimeType: "video/VP8", ClockRate: 90000}
			var referenceState, candidateState *sourceState
			// First close, quiet close without packets, then resume with a large
			// RTP time gap. The resume detects a missing jitter-window restoration.
			for cycle := 0; cycle < 3; cycle++ {
				dirs := []string{filepath.Join(root, fmt.Sprintf("reference-%d", cycle)), filepath.Join(root, fmt.Sprintf("candidate-%d", cycle))}
				for _, dir := range dirs {
					if err := os.MkdirAll(filepath.Join(dir, "sources"), 0o750); err != nil {
						t.Fatal(err)
					}
				}
				ref, err := newSourceWriterState(dirs[0], cycle, track, referenceState)
				if err != nil {
					t.Fatal(err)
				}
				candidate, err := newSourceWriterState(dirs[1], cycle, track, candidateState)
				if err != nil {
					t.Fatal(err)
				}
				input := packets
				if cycle == 1 {
					input = nil
				}
				if cycle == 2 {
					input = []*rtp.Packet{vp8(30, 180000, false, true, key[:7]), full(32, 225000), full(33, 270000), full(34, 273600)}
				}
				for i, packet := range input {
					at := int64(time.Second) + int64(cycle)*int64(time.Second) + int64(i)*int64(40*time.Millisecond)
					if err := ref.WriteRTP(packet.Clone(), at); err != nil {
						t.Fatal(err)
					}
					if err := candidate.WriteRTP(packet.Clone(), at); err != nil {
						t.Fatal(err)
					}
				}
				if ref.samples != candidate.samples || !reflect.DeepEqual(ref.state.preroll, candidate.state.preroll) {
					t.Fatalf("cycle%d jitter/sample output differs before final Flush", cycle)
				}
				ref.state.builder.Flush()
				if err := ref.pop(); err != nil {
					t.Fatal(err)
				}
				if err := ref.closeChunk(false); err != nil {
					t.Fatal(err)
				}
				if err := candidate.closeChunk(true); err != nil {
					t.Fatal(err)
				}
				left, err := os.ReadFile(ref.path)
				if err != nil {
					t.Fatal(err)
				}
				right, err := os.ReadFile(candidate.path)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(left, right) {
					t.Fatalf("cycle%d encoded bytes differ", cycle)
				}
				if ref.samples != candidate.samples || ref.firstAt != candidate.firstAt || ref.lastAt != candidate.lastAt || ref.lastDuration != candidate.lastDuration {
					t.Fatalf("cycle%d RTP timing/sample count changed", cycle)
				}
				if !reflect.DeepEqual(ref.state.arrivals, candidate.state.arrivals) || !reflect.DeepEqual(ref.state.preroll, candidate.state.preroll) || ref.state.bytes != candidate.state.bytes {
					t.Fatalf("cycle%d shared RTP state changed", cycle)
				}
				referenceState, candidateState = ref.state, candidate.state
			}
		})
	}
}
