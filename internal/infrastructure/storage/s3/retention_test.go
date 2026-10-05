package s3

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/records"
)

func TestRecordingDeletionRejectsUnsafeNamespaces(t *testing.T) {
	client := &Client{bucket: "recordings"}
	rid, cid := uuid.NewString(), uuid.NewString()
	otherBucket := "attachments"
	for _, record := range []records.Record{
		{UUID: ""}, {UUID: "../"}, {UUID: rid + "/../"},
		{UUID: rid, Mode: records.ModeComposite, ConferenceID: "../"},
		{UUID: rid, StorageBucket: &otherBucket},
	} {
		if err := client.RemoveRecording(context.Background(), record); err == nil {
			t.Fatal("unsafe deletion accepted", record)
		}
	}
	prefixes, err := recordingPrefixes(records.Record{UUID: rid, Mode: records.ModeComposite, ConferenceID: cid})
	if err != nil || len(prefixes) != 2 || prefixes[0] != "records/"+rid+"/" || prefixes[1] != "recordings/"+cid+"/"+rid+"/" {
		t.Fatal("incorrect deletion namespaces", prefixes, err)
	}
}
