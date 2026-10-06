package composite

import (
	"reflect"
	"testing"

	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	pionmedia "github.com/pion/webrtc/v4/pkg/media"
	"github.com/pion/webrtc/v4/pkg/media/samplebuilder"
)

// Missing the partition head is a normal RTP-loss/start-mid-frame path. The
// final orphan fragment must be dropped. The original flush scans empty slots
// repeatedly; the option guard changes no prepared samples or drop accounting.
func flushLossBuilder() *samplebuilder.SampleBuilder {
	builder := samplebuilder.New(64, &codecs.VP8Packet{}, 90000, samplebuilder.WithMaxTimeDelay(sourceJitterWindow))
	builder.Push(&rtp.Packet{Header: rtp.Header{SequenceNumber: 697, Timestamp: 994064077, Marker: true}, Payload: []byte{0, 0, 0, 0, 0x9d, 1, 0x2a, 0x80, 2, 0xe0, 1}})
	if builder.Pop() != nil {
		panic("orphan fragment unexpectedly prepared")
	}
	return builder
}

func BenchmarkBuilderFinalFlush(b *testing.B) {
	for _, disableScan := range []bool{false, true} {
		name := "existing"
		if disableScan {
			name = "without_redundant_time_scan"
		}
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				b.StopTimer()
				builder := flushLossBuilder()
				b.StartTimer()
				if disableScan {
					samplebuilder.WithMaxTimeDelay(0)(builder)
				}
				builder.Flush()
				if disableScan {
					samplebuilder.WithMaxTimeDelay(sourceJitterWindow)(builder)
				}
				b.StopTimer()
				if builder.Pop() != nil {
					b.Fatal("orphan fragment was emitted")
				}
			}
		})
	}
}

func TestFlushLossPreservesSamplesTimingAndDroppedPackets(t *testing.T) {
	ref, candidate := flushLossBuilder(), flushLossBuilder()
	ref.Flush()
	samplebuilder.WithMaxTimeDelay(0)(candidate)
	candidate.Flush()
	samplebuilder.WithMaxTimeDelay(sourceJitterWindow)(candidate)
	drain := func(b *samplebuilder.SampleBuilder) []*pionmedia.Sample {
		var out []*pionmedia.Sample
		for s := b.Pop(); s != nil; s = b.Pop() {
			out = append(out, s)
		}
		return out
	}
	if !reflect.DeepEqual(drain(ref), drain(candidate)) {
		t.Fatal("orphan output changed")
	}
	// Reuse after quiet source flush; include a complete keyframe, a packet gap
	// and a >200ms RTP-time jump. Compare complete Sample values, including
	// PacketTimestamp, Duration, PrevDroppedPackets and RTP headers.
	for _, builder := range []*samplebuilder.SampleBuilder{ref, candidate} {
		samplebuilder.WithRTPHeaders(true)(builder)
	}
	for _, p := range []rtp.Packet{
		{Header: rtp.Header{SequenceNumber: 698, Timestamp: 994067677, Marker: true}, Payload: []byte{0x10, 0, 0, 0, 0x9d, 1, 0x2a, 0x80, 2, 0xe0, 1}},
		{Header: rtp.Header{SequenceNumber: 700, Timestamp: 994112677, Marker: true}, Payload: []byte{0x10, 0, 0, 0, 0x9d, 1, 0x2a, 0x80, 2, 0xe0, 1}},
		{Header: rtp.Header{SequenceNumber: 701, Timestamp: 994157677, Marker: true}, Payload: []byte{0x10, 0, 0, 0, 0x9d, 1, 0x2a, 0x80, 2, 0xe0, 1}},
	} {
		ref.Push(p.Clone())
		candidate.Push(p.Clone())
		if !reflect.DeepEqual(drain(ref), drain(candidate)) {
			t.Fatal("sample/drop accounting changed before final flush")
		}
	}
	ref.Flush()
	samplebuilder.WithMaxTimeDelay(0)(candidate)
	candidate.Flush()
	samplebuilder.WithMaxTimeDelay(sourceJitterWindow)(candidate)
	if !reflect.DeepEqual(drain(ref), drain(candidate)) {
		t.Fatal("sample/drop accounting changed after final flush")
	}
}
