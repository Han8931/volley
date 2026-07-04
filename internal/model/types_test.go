package model

import (
	"encoding/base64"
	"testing"
)

func findHeader(hs []Header, name string) (Header, bool) {
	for _, h := range hs {
		if h.Name == name {
			return h, true
		}
	}
	return Header{}, false
}

func TestApplyAuthBearer(t *testing.T) {
	req := Request{Method: "GET", Auth: Auth{Type: AuthBearer, Token: "abc123"}}
	out := req.ApplyAuth()

	h, ok := findHeader(out.Headers, "Authorization")
	if !ok || h.Value != "Bearer abc123" || !h.Enabled {
		t.Fatalf("bearer header = %+v (found=%v), want {Authorization Bearer abc123 true}", h, ok)
	}
	// Source request must not be mutated.
	if len(req.Headers) != 0 {
		t.Errorf("ApplyAuth mutated the receiver's headers: %+v", req.Headers)
	}
}

func TestApplyAuthBasic(t *testing.T) {
	req := Request{Method: "GET", Auth: Auth{Type: AuthBasic, Username: "user", Password: "pass"}}
	out := req.ApplyAuth()

	want := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:pass"))
	h, ok := findHeader(out.Headers, "Authorization")
	if !ok || h.Value != want {
		t.Fatalf("basic header = %+v, want value %q", h, want)
	}
}

func TestApplyAuthAPIKeyHeader(t *testing.T) {
	req := Request{Method: "GET", Auth: Auth{Type: AuthAPIKey, Key: "X-API-Key", Value: "secret"}}
	out := req.ApplyAuth()

	h, ok := findHeader(out.Headers, "X-API-Key")
	if !ok || h.Value != "secret" || !h.Enabled {
		t.Fatalf("apikey header = %+v (found=%v), want {X-API-Key secret true}", h, ok)
	}
	if len(out.Query) != 0 {
		t.Errorf("apikey-in-header should not add a query param, got %+v", out.Query)
	}
}

func TestApplyAuthAPIKeyQuery(t *testing.T) {
	req := Request{Method: "GET", Auth: Auth{Type: AuthAPIKey, Key: "api_key", Value: "secret", InQuery: true}}
	out := req.ApplyAuth()

	if len(out.Query) != 1 || out.Query[0].Key != "api_key" || out.Query[0].Value != "secret" || !out.Query[0].Enabled {
		t.Fatalf("apikey query = %+v, want [{api_key secret true}]", out.Query)
	}
	if _, ok := findHeader(out.Headers, "api_key"); ok {
		t.Error("apikey-in-query should not add a header")
	}
}

func TestApplyAuthNoneAndEmpty(t *testing.T) {
	cases := []struct {
		name string
		auth Auth
	}{
		{"none", Auth{Type: AuthNone}},
		{"bearer empty token", Auth{Type: AuthBearer}},
		{"basic empty creds", Auth{Type: AuthBasic}},
		{"apikey empty name", Auth{Type: AuthAPIKey, Value: "x"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := Request{Method: "GET", Auth: c.auth}
			out := req.ApplyAuth()
			if len(out.Headers) != 0 || len(out.Query) != 0 {
				t.Errorf("expected no-op, got headers=%+v query=%+v", out.Headers, out.Query)
			}
		})
	}
}

func TestApplyAuthOverridesManualHeader(t *testing.T) {
	req := Request{
		Method:  "GET",
		Headers: []Header{{Name: "Authorization", Value: "Bearer stale", Enabled: true}},
		Auth:    Auth{Type: AuthBearer, Token: "fresh"},
	}
	out := req.ApplyAuth()

	// The injected header is appended after the manual one; httpx uses Header.Set,
	// so the last Authorization wins. Assert it is present and last.
	last := out.Headers[len(out.Headers)-1]
	if last.Name != "Authorization" || last.Value != "Bearer fresh" {
		t.Fatalf("last header = %+v, want the injected {Authorization Bearer fresh}", last)
	}
}
