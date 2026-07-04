package curl

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/tabularasa/volley/internal/model"
)

func TestParseSimpleGET(t *testing.T) {
	req, warns, err := Parse("curl https://api.test/users")
	if err != nil {
		t.Fatal(err)
	}
	if req.Method != "GET" || req.URL != "https://api.test/users" {
		t.Errorf("got %+v", req)
	}
	if len(warns) != 0 {
		t.Errorf("unexpected warnings: %v", warns)
	}
}

func TestParsePostWithHeadersAndData(t *testing.T) {
	req, _, err := Parse(`curl -X POST https://api.test/login -H 'Content-Type: application/json' -H "Accept: */*" -d '{"u":"a"}'`)
	if err != nil {
		t.Fatal(err)
	}
	if req.Method != "POST" || req.Body != `{"u":"a"}` {
		t.Errorf("method/body = %q / %q", req.Method, req.Body)
	}
	if len(req.Headers) != 2 || req.Headers[0] != (model.Header{Name: "Content-Type", Value: "application/json", Enabled: true}) {
		t.Errorf("headers = %+v", req.Headers)
	}
	if req.Headers[1].Name != "Accept" || req.Headers[1].Value != "*/*" {
		t.Errorf("second header = %+v", req.Headers[1])
	}
}

func TestParseMethodInferredFromData(t *testing.T) {
	req, _, err := Parse(`curl https://api.test --data-raw 'x=1'`)
	if err != nil {
		t.Fatal(err)
	}
	if req.Method != "POST" {
		t.Errorf("a body without -X should imply POST, got %q", req.Method)
	}
}

// The format browsers emit via "Copy as cURL": quoted URL, repeated -H, a
// --data-raw payload, backslash line continuations, and a --compressed flag.
func TestParseBrowserCopyAsCurl(t *testing.T) {
	in := `curl 'https://api.test/graphql' \
  -H 'authorization: Bearer xyz' \
  -H 'content-type: application/json' \
  --data-raw '{"query":"{me}"}' \
  --compressed`
	req, warns, err := Parse(in)
	if err != nil {
		t.Fatal(err)
	}
	if req.Method != "POST" || req.URL != "https://api.test/graphql" {
		t.Errorf("got method=%q url=%q", req.Method, req.URL)
	}
	if len(req.Headers) != 2 || req.Body != `{"query":"{me}"}` {
		t.Errorf("headers=%+v body=%q", req.Headers, req.Body)
	}
	if len(warns) != 0 {
		t.Errorf("--compressed should be silently ignored, got warnings %v", warns)
	}
}

func TestParseBasicAuth(t *testing.T) {
	req, _, err := Parse(`curl -u alice:secret https://api.test`)
	if err != nil {
		t.Fatal(err)
	}
	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("alice:secret"))
	if len(req.Headers) != 1 || req.Headers[0].Name != "Authorization" || req.Headers[0].Value != want {
		t.Errorf("basic auth header = %+v", req.Headers)
	}
}

func TestParseAttachedAndInlineForms(t *testing.T) {
	// -XPOST (attached short) and --header=... (inline long).
	req, _, err := Parse(`curl -XPOST --header=X-A:1 --max-time 5 https://api.test`)
	if err != nil {
		t.Fatal(err)
	}
	if req.Method != "POST" {
		t.Errorf("attached -XPOST not parsed, got %q", req.Method)
	}
	if len(req.Headers) != 1 || req.Headers[0].Name != "X-A" || req.Headers[0].Value != "1" {
		t.Errorf("inline header = %+v", req.Headers)
	}
	if req.Timeout != 5*time.Second {
		t.Errorf("timeout = %v, want 5s", req.Timeout)
	}
}

func TestParseUnknownFlagWarns(t *testing.T) {
	_, warns, err := Parse(`curl --frobnicate https://api.test`)
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "frobnicate") {
		t.Errorf("expected a warning about --frobnicate, got %v", warns)
	}
}

func TestParseUnsupportedValueFlagConsumesValue(t *testing.T) {
	req, warns, err := Parse(`curl --connect-timeout 10 https://api.test`)
	if err != nil {
		t.Fatal(err)
	}
	if req.URL != "https://api.test" {
		t.Errorf("URL = %q, want real URL after unsupported flag value", req.URL)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "connect-timeout") {
		t.Errorf("expected connect-timeout warning, got %v", warns)
	}
}

func TestParseErrors(t *testing.T) {
	if _, _, err := Parse(`curl -X POST -H 'a: b'`); err == nil {
		t.Error("a command with no URL should error")
	}
	if _, _, err := Parse(`curl 'unterminated`); err == nil {
		t.Error("an unterminated quote should error")
	}
}

func TestFormatGET(t *testing.T) {
	out := Format(model.Request{Method: "GET", URL: "https://api.test/x"})
	if strings.Contains(out, "-X") {
		t.Errorf("GET should omit -X, got %q", out)
	}
	if !strings.Contains(out, "'https://api.test/x'") {
		t.Errorf("URL should be quoted, got %q", out)
	}
}

func TestFormatPostAllFields(t *testing.T) {
	out := Format(model.Request{
		Method:  "POST",
		URL:     "https://api.test",
		Headers: []model.Header{{Name: "Accept", Value: "application/json", Enabled: true}, {Name: "X-Off", Value: "no", Enabled: false}},
		Body:    `{"a":1}`,
		Timeout: 30 * time.Second,
	})
	for _, want := range []string{"-X POST", "-H 'Accept: application/json'", `--data '{"a":1}'`, "--max-time 30"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "X-Off") {
		t.Errorf("disabled header should be excluded, got:\n%s", out)
	}
}

func TestFormatQuoteEscaping(t *testing.T) {
	out := Format(model.Request{Method: "GET", URL: "https://api.test/o'brien"})
	if !strings.Contains(out, `'https://api.test/o'\''brien'`) {
		t.Errorf("single quotes should be shell-escaped, got %q", out)
	}
}

func TestRoundTrip(t *testing.T) {
	orig := model.Request{
		Method:  "PUT",
		URL:     "https://api.test/items/42",
		Headers: []model.Header{{Name: "Content-Type", Value: "application/json", Enabled: true}},
		Body:    `{"name":"x"}`,
		Timeout: 15 * time.Second,
	}
	got, warns, err := Parse(Format(orig))
	if err != nil {
		t.Fatal(err)
	}
	if len(warns) != 0 {
		t.Errorf("round-trip warnings: %v", warns)
	}
	if got.Method != orig.Method || got.URL != orig.URL || got.Body != orig.Body || got.Timeout != orig.Timeout {
		t.Errorf("round-trip mismatch:\n got  %+v\n want %+v", got, orig)
	}
	if len(got.Headers) != 1 || got.Headers[0] != orig.Headers[0] {
		t.Errorf("round-trip headers = %+v", got.Headers)
	}
}
