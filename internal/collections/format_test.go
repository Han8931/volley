package collections

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tabularasa/volley/internal/model"
)

// The on-disk JSON must use the versioned, human-readable schema: a
// schemaVersion stamp, lower-cased keys, and a duration string (not raw ns).
func TestSaveWritesVersionedHumanReadableFormat(t *testing.T) {
	s := Store{Root: t.TempDir()}
	req := model.Request{
		Method:  "POST",
		URL:     "https://example.test",
		Headers: []model.Header{{Name: "Accept", Value: "application/json", Enabled: true}},
		Body:    `{"ok":true}`,
		Timeout: 12 * time.Second,
	}
	if err := s.Save("api/create", req); err != nil {
		t.Fatalf("Save: %v", err)
	}

	b, err := os.ReadFile(filepath.Join(s.Root, "api", "create.json"))
	if err != nil {
		t.Fatalf("read saved file: %v", err)
	}
	text := string(b)
	for _, want := range []string{`"schemaVersion": 1`, `"timeout": "12s"`, `"method": "POST"`, `"name": "Accept"`} {
		if !strings.Contains(text, want) {
			t.Errorf("saved file missing %q; got:\n%s", want, text)
		}
	}
	if strings.Contains(text, "12000000000") {
		t.Errorf("timeout should be a duration string, not raw nanoseconds; got:\n%s", text)
	}
}

// A request written in the pre-versioned format (capitalized keys, timeout as
// nanoseconds, no schemaVersion) must still load. Guards against breaking files
// saved by earlier builds.
func TestLoadAcceptsLegacyFormat(t *testing.T) {
	s := Store{Root: t.TempDir()}
	if err := os.MkdirAll(s.Root, 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := `{
  "Method": "PUT",
  "URL": "https://legacy.test/a",
  "Headers": [{"Name": "X-Old", "Value": "1", "Enabled": true}],
  "Query": [{"Key": "q", "Value": "2", "Enabled": true}],
  "Body": "hi",
  "Timeout": 5000000000
}`
	if err := os.WriteFile(filepath.Join(s.Root, "old.json"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := s.Load("old")
	if err != nil {
		t.Fatalf("Load legacy: %v", err)
	}
	want := model.Request{
		Method:  "PUT",
		URL:     "https://legacy.test/a",
		Headers: []model.Header{{Name: "X-Old", Value: "1", Enabled: true}},
		Query:   []model.KV{{Key: "q", Value: "2", Enabled: true}},
		Body:    "hi",
		Timeout: 5 * time.Second,
	}
	if got.Method != want.Method || got.URL != want.URL || got.Body != want.Body ||
		got.Timeout != want.Timeout || len(got.Headers) != 1 || got.Headers[0] != want.Headers[0] ||
		len(got.Query) != 1 || got.Query[0] != want.Query[0] {
		t.Fatalf("legacy load mismatch:\n got  %+v\n want %+v", got, want)
	}
}

// A duration string round-trips through JSON; a zero timeout is omitted.
func TestDurationMarshaling(t *testing.T) {
	b, err := json.Marshal(storedRequest{SchemaVersion: 1, Method: "GET", Timeout: duration(90 * time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"timeout":"1m30s"`) {
		t.Errorf("want timeout string 1m30s, got %s", b)
	}
	b, err = json.Marshal(storedRequest{SchemaVersion: 1, Method: "GET"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "timeout") {
		t.Errorf("zero timeout should be omitted, got %s", b)
	}
}

// Save must not leave temp files behind, and must overwrite atomically.
func TestSaveLeavesNoTempFiles(t *testing.T) {
	s := Store{Root: t.TempDir()}
	if err := s.Save("dir/req", model.Request{Method: "GET", URL: "https://a.test"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Save("dir/req", model.Request{Method: "POST", URL: "https://b.test"}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(s.Root, "dir"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") || strings.HasPrefix(e.Name(), ".volley-") {
			t.Errorf("leftover temp file: %s", e.Name())
		}
	}
	got, _ := s.Load("dir/req")
	if got.Method != "POST" || got.URL != "https://b.test" {
		t.Errorf("overwrite not applied: %+v", got)
	}
}
