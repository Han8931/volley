package build

import (
	"testing"

	"github.com/tabularasa/volley/internal/model"
	"github.com/tabularasa/volley/internal/vars"
)

func TestAppendQuery(t *testing.T) {
	got := AppendQuery("https://x.test/api", []model.KV{
		{Key: "a", Value: "1", Enabled: true},
		{Key: "skip", Value: "9", Enabled: false},
	})
	if got != "https://x.test/api?a=1" {
		t.Errorf("AppendQuery = %q", got)
	}
}

func TestFinalPipeline(t *testing.T) {
	req := model.Request{
		Method: "GET",
		URL:    "https://{{host}}/v1",
		Query:  []model.KV{{Key: "q", Value: "{{term}}", Enabled: true}},
		Auth:   model.Auth{Type: model.AuthBearer, Token: "{{tok}}"},
	}
	resolver := vars.Layered{map[string]string{
		"host": "api.test", "term": "x y", "tok": "t0k",
	}}
	out := Final(req, resolver)

	if out.URL != "https://api.test/v1?q=x+y" {
		t.Errorf("URL = %q", out.URL)
	}
	var auth string
	for _, h := range out.Headers {
		if h.Name == "Authorization" {
			auth = h.Value
		}
	}
	if auth != "Bearer t0k" {
		t.Errorf("Authorization = %q", auth)
	}
}
