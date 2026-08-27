package candidates

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMergesCustomWordlistAndDeduplicates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "params.txt")
	if err := os.WriteFile(path, []byte("# comment\norganization_id\ncustom_param\ncustom_param\n"), 0600); err != nil {
		t.Fatal(err)
	}
	words, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, w := range words {
		seen[w]++
	}
	if seen["organization_id"] != 1 || seen["custom_param"] != 1 {
		t.Fatalf("unexpected candidates: %+v", seen)
	}
}
