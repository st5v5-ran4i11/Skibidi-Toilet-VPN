package checker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/model"
)

func tvResults(count int) []model.FinalResult {
	out := make([]model.FinalResult, 0, count)
	for i := 0; i < count; i++ {
		out = append(out, model.FinalResult{Alive: true, Score: count - i, Node: model.NodeInfo{
			Raw:         "vless://user@host" + itoa(i) + ".example:443?encryption=none",
			DisplayName: "node" + itoa(i),
		}})
	}
	return out
}

func countLines(t *testing.T, path string) int {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) == 0 {
		return 0
	}
	return len(strings.Split(strings.TrimSuffix(string(b), "\n"), "\n"))
}

func TestWriteTVPoolLimitsAndEmpty(t *testing.T) {
	cases := []struct {
		name        string
		count, want int
	}{
		{"empty", 0, 0}, {"under", 37, 37}, {"exact", 100, 100}, {"over", 145, 100},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := writeTVPool(dir, tvResults(tc.count)); err != nil {
				t.Fatal(err)
			}
			if got := countLines(t, filepath.Join(dir, "tv.txt")); got != tc.want {
				t.Fatalf("got %d lines, want %d", got, tc.want)
			}
		})
	}
}

func TestWriteTVPoolFiltersFailedAndDeduplicates(t *testing.T) {
	dir := t.TempDir()
	rows := []model.FinalResult{
		{Alive: true, Node: model.NodeInfo{Raw: "vless://a@same.example:443?x=1"}},
		{Alive: true, Node: model.NodeInfo{Raw: "trojan://b@same.example:443?x=2"}},
		{Alive: false, Node: model.NodeInfo{Raw: "vless://a@dead.example:443"}},
		{Alive: true, Node: model.NodeInfo{Raw: "vless://a@unique.example:443"}},
	}
	if err := writeTVPool(dir, rows); err != nil {
		t.Fatal(err)
	}
	if got := countLines(t, filepath.Join(dir, "tv.txt")); got != 2 {
		t.Fatalf("got %d lines, want 2", got)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "tv.txt"))
	if strings.Contains(string(data), "dead.example") {
		t.Fatal("failed node was written")
	}
}

func TestWriteTVPoolKeepsPreviousFileOnWriteError(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "tv.txt")
	if err := os.Mkdir(target, 0755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(target, "marker")
	if err := os.WriteFile(marker, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := writeTVPool(dir, tvResults(1)); err == nil {
		t.Fatal("expected rename failure")
	}
	b, err := os.ReadFile(marker)
	if err != nil || string(b) != "keep" {
		t.Fatal("existing target was altered")
	}
}

func TestTVPoolUsesRankingOrder(t *testing.T) {
	dir := t.TempDir()
	rows := []model.FinalResult{
		{Alive: true, Score: 10, Node: model.NodeInfo{Raw: "vless://a@slow.example:443"}},
		{Alive: true, Score: 90, Node: model.NodeInfo{Raw: "vless://a@fast.example:443"}},
	}
	sortFinalResults(rows)
	if err := writeTVPool(dir, rows); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "tv.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "vless://a@fast.example:443") {
		t.Fatalf("top-ranked node was not written first: %s", data)
	}
}
