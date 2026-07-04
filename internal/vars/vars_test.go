package vars

import (
	"reflect"
	"testing"

	"github.com/tabularasa/volley/internal/model"
)

func TestExpand(t *testing.T) {
	s := Store{"host": "api.test", "ver": "v2"}
	got := s.Expand("https://{{host}}/{{ ver }}/ping")
	if got != "https://api.test/v2/ping" {
		t.Errorf("Expand = %q", got)
	}
	if got := s.Expand("{{unknown}}"); got != "{{unknown}}" {
		t.Errorf("unknown placeholder should be preserved, got %q", got)
	}
}

func TestExpandEnvFallback(t *testing.T) {
	t.Setenv("VOLLEY_TEST_TOKEN", "xyz")
	s := New()
	if got := s.Expand("Bearer {{VOLLEY_TEST_TOKEN}}"); got != "Bearer xyz" {
		t.Errorf("env fallback = %q", got)
	}
	// store takes precedence over the environment.
	s.Set("VOLLEY_TEST_TOKEN", "override")
	if got := s.Expand("{{VOLLEY_TEST_TOKEN}}"); got != "override" {
		t.Errorf("store should win over env, got %q", got)
	}
}

func TestApply(t *testing.T) {
	s := Store{"tok": "secret", "p": "users"}
	req := model.Request{
		URL:     "https://x.test/{{p}}",
		Headers: []model.Header{{Name: "Authorization", Value: "Bearer {{tok}}", Enabled: true}},
		Query:   []model.KV{{Key: "for", Value: "{{p}}", Enabled: true}},
		Body:    `{"who":"{{p}}"}`,
	}
	out := s.Apply(req)
	if out.URL != "https://x.test/users" {
		t.Errorf("URL = %q", out.URL)
	}
	if out.Headers[0].Value != "Bearer secret" {
		t.Errorf("header = %q", out.Headers[0].Value)
	}
	if out.Query[0].Value != "users" {
		t.Errorf("query = %q", out.Query[0].Value)
	}
	if out.Body != `{"who":"users"}` {
		t.Errorf("body = %q", out.Body)
	}
}

func TestUnresolved(t *testing.T) {
	// A fully-resolved request reports nothing.
	if got := Unresolved(model.Request{URL: "https://x.test/ok", Body: "done"}); len(got) != 0 {
		t.Errorf("resolved request should report no missing vars, got %v", got)
	}

	// Leftover placeholders are collected across URL, body and enabled headers,
	// de-duplicated and sorted; disabled headers are ignored (they aren't sent).
	req := model.Request{
		URL:  "https://{{host}}/{{path}}",
		Body: "token={{token}} and {{host}}",
		Headers: []model.Header{
			{Name: "X-Api", Value: "{{token}}", Enabled: true},
			{Name: "X-Skip", Value: "{{ignored}}", Enabled: false},
		},
		Query: []model.KV{
			{Key: "{{qkey}}", Value: "{{qval}}", Enabled: true},
			{Key: "off", Value: "{{skipq}}", Enabled: false},
		},
	}
	want := []string{"host", "path", "qkey", "qval", "token"}
	if got := Unresolved(req); !reflect.DeepEqual(got, want) {
		t.Errorf("Unresolved = %v, want %v", got, want)
	}
}

func TestApplyExpandsAuth(t *testing.T) {
	s := Store{"tok": "secret", "u": "admin", "pw": "hunter2", "k": "keyval"}

	bearer := s.Apply(model.Request{Auth: model.Auth{Type: model.AuthBearer, Token: "{{tok}}"}})
	if bearer.Auth.Token != "secret" {
		t.Errorf("bearer token = %q, want secret", bearer.Auth.Token)
	}

	basic := s.Apply(model.Request{Auth: model.Auth{Type: model.AuthBasic, Username: "{{u}}", Password: "{{pw}}"}})
	if basic.Auth.Username != "admin" || basic.Auth.Password != "hunter2" {
		t.Errorf("basic = %q/%q, want admin/hunter2", basic.Auth.Username, basic.Auth.Password)
	}

	apikey := s.Apply(model.Request{Auth: model.Auth{Type: model.AuthAPIKey, Key: "X-Key", Value: "{{k}}"}})
	if apikey.Auth.Value != "keyval" {
		t.Errorf("apikey value = %q, want keyval", apikey.Auth.Value)
	}
}

func TestUnresolvedIncludesAuthByType(t *testing.T) {
	// Only the active type's fields are inspected.
	req := model.Request{Auth: model.Auth{
		Type:     model.AuthBearer,
		Token:    "{{tok}}",
		Username: "{{unused}}", // belongs to Basic; must be ignored
	}}
	if got := Unresolved(req); !reflect.DeepEqual(got, []string{"tok"}) {
		t.Errorf("Unresolved = %v, want [tok]", got)
	}
}
