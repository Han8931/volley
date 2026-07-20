package codegen

import (
	"strings"
	"testing"
	"time"

	"github.com/tabularasa/volley/internal/model"
)

var sample = model.Request{
	Method: "POST",
	URL:    "https://api.test/v1?q=x",
	Headers: []model.Header{
		{Name: "Authorization", Value: "Bearer t0k", Enabled: true},
		{Name: "X-Skip", Value: "no", Enabled: false},
	},
	Body:    `{"a":"it's"}`,
	Timeout: 10 * time.Second,
}

func TestGenerateFormats(t *testing.T) {
	for _, format := range Formats {
		out, err := Generate(format, sample)
		if err != nil {
			t.Fatalf("%s: %v", format, err)
		}
		if strings.Contains(out, "X-Skip") {
			t.Errorf("%s must omit disabled headers:\n%s", format, out)
		}
		if !strings.Contains(out, `'{"a":"it'\''s"}'`) {
			t.Errorf("%s should shell-quote the body:\n%s", format, out)
		}
	}
	if _, err := Generate("carrier-pigeon", sample); err == nil {
		t.Error("unknown format should error")
	}
}

func TestWgetShape(t *testing.T) {
	out := Wget(sample)
	for _, want := range []string{"wget -qO-", "--method=POST", "--header='Authorization: Bearer t0k'", "--timeout=10", "'https://api.test/v1?q=x'"} {
		if !strings.Contains(out, want) {
			t.Errorf("wget missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(Wget(model.Request{Method: "GET", URL: "https://x.test"}), "--method") {
		t.Error("plain GET needs no --method")
	}
}

func TestHTTPieShape(t *testing.T) {
	out := HTTPie(sample)
	for _, want := range []string{"http --timeout=10 POST", "'Authorization:Bearer t0k'", "--raw"} {
		if !strings.Contains(out, want) {
			t.Errorf("httpie missing %q:\n%s", want, out)
		}
	}
	plain := HTTPie(model.Request{Method: "GET", URL: "https://x.test"})
	if strings.Contains(plain, " GET ") {
		t.Errorf("httpie omits GET for plain requests: %s", plain)
	}
}
