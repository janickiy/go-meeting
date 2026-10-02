package search

import (
	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/config"
	content "github.com/janickiy/go-recorder/internal/domain/content"
	"math"
	"strings"
	"testing"
	"unicode/utf8"
)

// TestChunkIdentityAndModelIsolation проверяет Unicode, timestamps, поколение и несовместимость моделей.
// @args t — исполнитель теста.
func TestChunkIdentityAndModelIsolation(t *testing.T) {
	cfg := config.StageEightConfig{EmbeddingModel: "test", EmbeddingVersion: "v1", EmbeddingDimensions: 64, EmbeddingChunkRunes: 128}
	transcript := content.Transcript{ID: uuid.NewString(), Generation: 1}
	segments := []content.Segment{{ID: uuid.NewString(), Text: strings.Repeat("я", 300), StartMS: 5, EndMS: 1000}}
	a, err := Chunks(transcript, segments, cfg)
	if err != nil || len(a) != 3 {
		t.Fatal("chunking", err)
	}
	b, _ := Chunks(transcript, segments, cfg)
	if a[0].ID != b[0].ID || a[0].ContentHash != b[0].ContentHash || !utf8.ValidString(a[0].Text) || a[0].StartMS != 5 {
		t.Fatal("unstable metadata")
	}
	cfg.EmbeddingVersion = "v2"
	b, _ = Chunks(transcript, segments, cfg)
	if a[0].ID == b[0].ID || a[0].ModelKey == b[0].ModelKey || a[0].ContentHash != b[0].ContentHash {
		t.Fatal("model mixing")
	}
	transcript.Generation++
	b, _ = Chunks(transcript, segments, cfg)
	if b[0].Generation != 2 {
		t.Fatal("generation lost")
	}
}

// TestEmbeddingVectorValidation исключает несовместимые размеры и недопустимые значения.
// @args t — исполнитель теста.
func TestEmbeddingVectorValidation(t *testing.T) {
	if !ValidVectors([][]float32{{1, 0}}, 1, 2) {
		t.Fatal("valid vector")
	}
	for _, v := range [][][]float32{nil, {{0, 0}}, {{1}}, {{float32(math.Inf(1)), 0}}, {{float32(math.NaN()), 1}}} {
		if ValidVectors(v, 1, 2) {
			t.Fatal("invalid vector accepted")
		}
	}
}
