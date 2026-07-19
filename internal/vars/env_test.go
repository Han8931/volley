package vars

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/tabularasa/volley/internal/model"
)

func TestEnvStoreRoundTrip(t *testing.T) {
	s := EnvStore{Root: filepath.Join(t.TempDir(), "environments")}

	if names, err := s.List(); err != nil || len(names) != 0 {
		t.Fatalf("empty store: names=%v err=%v", names, err)
	}

	want := map[string]string{"base_url": "https://staging.test", "token": "s3cret"}
	if err := s.Save("staging", want); err != nil {
		t.Fatal(err)
	}
	if err := s.Save("prod", map[string]string{"base_url": "https://prod.test"}); err != nil {
		t.Fatal(err)
	}

	got, err := s.Load("staging")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Load = %v, want %v", got, want)
	}

	names, err := s.List()
	if err != nil || !reflect.DeepEqual(names, []string{"prod", "staging"}) {
		t.Errorf("List = %v (err %v), want sorted [prod staging]", names, err)
	}

	if err := s.Delete("prod"); err != nil {
		t.Fatal(err)
	}
	if names, _ := s.List(); !reflect.DeepEqual(names, []string{"staging"}) {
		t.Errorf("after delete List = %v", names)
	}

	// Environment files hold tokens — they must not be world-readable.
	info, err := os.Stat(filepath.Join(s.Root, "staging.json"))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("env file mode = %o, want 600", perm)
	}
}

func TestEnvStoreRejectsBadNames(t *testing.T) {
	s := EnvStore{Root: t.TempDir()}
	for _, name := range []string{"", "  ", "../escape", "/abs"} {
		if err := s.Save(name, nil); err == nil {
			t.Errorf("Save(%q) should fail", name)
		}
	}
}

func TestLayeredPrecedence(t *testing.T) {
	t.Setenv("VLY_OS_ONLY", "from-os")
	t.Setenv("VLY_SHADOWED", "from-os")

	session := Store{"tok": "session-tok", "VLY_SHADOWED": "from-session"}
	env := map[string]string{"tok": "env-tok", "host": "env-host"}
	l := Layered{session, env}

	got := l.Expand("{{tok}} {{host}} {{VLY_OS_ONLY}} {{VLY_SHADOWED}} {{missing}}")
	want := "session-tok env-host from-os from-session {{missing}}"
	if got != want {
		t.Errorf("Expand = %q, want %q", got, want)
	}

	req := model.Request{URL: "https://{{host}}/x", Body: "{{tok}}"}
	out := l.Apply(req)
	if out.URL != "https://env-host/x" || out.Body != "session-tok" {
		t.Errorf("Apply = %+v", out)
	}

	if v, ok := l.Lookup("host"); !ok || v != "env-host" {
		t.Errorf("Lookup(host) = %q, %v", v, ok)
	}
	if _, ok := l.Lookup("missing"); ok {
		t.Error("Lookup(missing) should report absence")
	}
}
