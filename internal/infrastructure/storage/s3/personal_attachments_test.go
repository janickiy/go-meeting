package s3

import (
	"github.com/google/uuid"
	"testing"
)

func TestAttachmentScopes(t *testing.T) {
	ids := []string{uuid.NewString(), uuid.NewString(), uuid.NewString()}
	for _, scope := range []string{"attachments/", "attachments/direct/"} {
		key := scope + ids[0] + "/" + ids[1] + "/" + ids[2]
		if !attachmentKey(key) {
			t.Fatal("valid scope rejected")
		}
		for _, bad := range []string{scope + "../" + ids[1] + "/" + ids[2], key + "/extra", "other/" + key, key + "?token=x", "attachments/direct/direct/" + ids[0] + "/" + ids[1] + "/" + ids[2]} {
			if attachmentKey(bad) {
				t.Fatal("unsafe object key accepted")
			}
		}
	}
}
