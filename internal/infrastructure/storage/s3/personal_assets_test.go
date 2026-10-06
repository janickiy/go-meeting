package s3

import "testing"

func TestPersonalAvatarKeyIsolation(t *testing.T) {
	id := "11111111-1111-4111-8111-111111111111"
	valid := "avatars/conversations/" + id + "/" + id
	if !personalAvatarKey(valid) {
		t.Fatal("valid private key denied")
	}
	for _, key := range []string{"avatars/conferences/" + id + "/" + id, valid + "/x", valid + "?token=x", "avatars/conversations/../" + id, "attachments/direct/" + id + "/" + id + "/" + id, "records/" + id} {
		if personalAvatarKey(key) {
			t.Fatalf("unsafe key accepted: %s", key)
		}
	}
}
