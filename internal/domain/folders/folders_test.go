package folders

import (
	"strings"
	"testing"
)

func TestFolderNameNormalization(t *testing.T) {
	name, key, err := NormalizeName("  Cafe\u0301  ")
	if err != nil || name != "Café" || key != "café" {
		t.Fatalf("normalization: %q %q %v", name, key, err)
	}
	_, fold, _ := NormalizeName("STRASSE")
	_, expanded, _ := NormalizeName("Straße")
	if fold != expanded {
		t.Fatal("Unicode case folding did not match")
	}
	if _, _, err = NormalizeName(strings.Repeat("я", 50)); err != nil {
		t.Fatal(err)
	}
	if _, _, err = NormalizeName("👩‍💻"); err != nil {
		t.Fatal("joiner rejected", err)
	}
	for _, invalid := range []string{"", "   ", strings.Repeat("я", 51), "a\nb", "a\x00b", "a\u202eb", "\xff"} {
		if _, _, err = NormalizeName(invalid); err == nil {
			t.Fatalf("accepted %q", invalid)
		}
	}
}
func TestFolderFilterValidation(t *testing.T) {
	filter, err := NormalizeFilter(Filter{Search: "  Café  "})
	if err != nil || filter.Type != "all" || filter.Search != "Café" {
		t.Fatal(filter, err)
	}
	for _, invalid := range []Filter{{Type: "group"}, {Search: strings.Repeat("x", 101)}, {Search: "a\u2066b"}} {
		if _, err = NormalizeFilter(invalid); err == nil {
			t.Fatal("invalid filter accepted", invalid)
		}
	}
}
