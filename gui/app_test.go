package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tabularasa/volley/internal/collections"
	"github.com/tabularasa/volley/internal/loadtest"
	"github.com/tabularasa/volley/internal/vars"
)

func testApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	return newApp(
		collections.Store{Root: filepath.Join(dir, "collections")},
		vars.EnvStore{Root: filepath.Join(dir, "environments")},
		loadtest.Store{Root: filepath.Join(dir, "loadprofiles")},
		loadtest.ResultStore{Root: filepath.Join(dir, "loadresults")},
	)
}

func TestSendResolvesVarsAndAuth(t *testing.T) {
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()

	a := testApp(t)
	if err := a.envStore.Save("staging", map[string]string{"tok": "t0k"}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.UseEnvironment("staging"); err != nil {
		t.Fatal(err)
	}

	resp := a.Send(RequestDTO{
		Method: "GET",
		URL:    srv.URL + "/v1",
		Query:  []KVDTO{{Key: "q", Value: "x", Enabled: true}},
		Auth:   AuthDTO{Type: "bearer", Token: "{{tok}}"},
	})
	if resp.Error != "" {
		t.Fatalf("Send error: %s", resp.Error)
	}
	if resp.StatusCode != 200 || resp.Body != `{"ok":true}` {
		t.Errorf("resp = %+v", resp)
	}
	if gotAuth != "Bearer t0k" {
		t.Errorf("Authorization = %q — env var not resolved", gotAuth)
	}
	if gotPath != "/v1?q=x" {
		t.Errorf("path = %q — query not folded", gotPath)
	}
	if resp.FinalURL != srv.URL+"/v1?q=x" {
		t.Errorf("FinalURL = %q", resp.FinalURL)
	}
}

func TestUnresolvedReportsMissing(t *testing.T) {
	a := testApp(t)
	missing := a.Unresolved(RequestDTO{Method: "GET", URL: "https://{{host}}/x"})
	if len(missing) != 1 || missing[0] != "host" {
		t.Errorf("Unresolved = %v", missing)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	a := testApp(t)
	in := RequestDTO{
		Method:  "POST",
		URL:     "https://x.test",
		Headers: []HeaderDTO{{Name: "X-A", Value: "1", Enabled: true}},
		Body:    `{"a":1}`,
	}
	if err := a.SaveRequest("grp/one", in); err != nil {
		t.Fatal(err)
	}
	got, err := a.LoadRequest("grp/one")
	if err != nil {
		t.Fatal(err)
	}
	if got.Method != "POST" || got.Body != in.Body || len(got.Headers) != 1 {
		t.Errorf("LoadRequest = %+v", got)
	}
	items, err := a.ListRequests()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, it := range items {
		names = append(names, it.Name)
	}
	if len(items) != 2 { // the group dir + the request
		t.Errorf("ListRequests = %v", names)
	}
}

func TestCurlRoundTrip(t *testing.T) {
	a := testApp(t)
	got, err := a.ImportCurl(`curl -X POST https://x.test/v1 -H 'X-A: 1' -d '{"a":1}'`)
	if err != nil {
		t.Fatal(err)
	}
	r := got.Request
	if r.Method != "POST" || r.URL != "https://x.test/v1" || r.Body != `{"a":1}` || len(r.Headers) != 1 {
		t.Errorf("ImportCurl = %+v", r)
	}

	a.SetSessionVar("host", "y.test")
	out := a.ExportCurl(RequestDTO{Method: "GET", URL: "https://{{host}}/z"})
	if !strings.Contains(out, "https://y.test/z") {
		t.Errorf("ExportCurl should expand vars, got %q", out)
	}
}

func TestSessionVars(t *testing.T) {
	a := testApp(t)
	a.SetSessionVar("tok", "abc")
	if got := a.SessionVars(); got["tok"] != "abc" {
		t.Errorf("SessionVars = %v", got)
	}
	a.SetSessionVar("tok", "") // empty value removes the override
	if got := a.SessionVars(); len(got) != 0 {
		t.Errorf("after clearing, SessionVars = %v", got)
	}
}

func TestEnvironmentEditing(t *testing.T) {
	a := testApp(t)
	st, err := a.SaveEnvironment("dev", map[string]string{"a": "1"})
	if err != nil || st.Active != "dev" {
		t.Fatalf("SaveEnvironment: %+v, %v", st, err)
	}
	vals, err := a.GetEnvironment("dev")
	if err != nil || vals["a"] != "1" {
		t.Errorf("GetEnvironment = %v, %v", vals, err)
	}
	st, err = a.DeleteEnvironment("dev")
	if err != nil || st.Active != "" || len(st.Names) != 0 {
		t.Errorf("DeleteEnvironment: %+v, %v", st, err)
	}
}

func TestLoadTestLifecycle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := testApp(t)
	quick := ProfileDTO{
		Name:   "quick",
		Points: []PointDTO{{AtMS: 0, RPS: 40}, {AtMS: 300, RPS: 40}},
	}
	if err := a.SaveProfile("quick", quick); err != nil {
		t.Fatal(err)
	}

	if err := a.StartLoadTest("quick", RequestDTO{Method: "GET", URL: srv.URL}); err != nil {
		t.Fatal(err)
	}
	if err := a.StartLoadTest("quick", RequestDTO{Method: "GET", URL: srv.URL}); err == nil {
		t.Error("second start while running should be refused")
	}

	deadline := time.After(5 * time.Second)
	var st LoadRunDTO
	for {
		st = a.PollLoadTest()
		if st.Done {
			break
		}
		select {
		case <-deadline:
			t.Fatal("run never finished")
		case <-time.After(50 * time.Millisecond):
		}
	}
	if st.Completed == 0 || st.Errors != 0 || st.OK != st.Completed {
		t.Errorf("finished run: %+v", st)
	}
	if st.Summary == nil || st.SummaryText == "" || st.Summary.Profile != "quick" {
		t.Errorf("summary missing: %+v", st.Summary)
	}
	if st.SavedAs == "" || !strings.HasPrefix(st.SavedAs, "quick-") {
		t.Errorf("result not auto-saved: savedAs=%q saveErr=%q", st.SavedAs, st.SaveError)
	}
	if len(st.Buckets) == 0 {
		t.Error("buckets missing — charts would be empty")
	}

	// The summary is built and saved exactly once across polls.
	again := a.PollLoadTest()
	if again.SavedAs != st.SavedAs {
		t.Errorf("second poll re-saved: %q vs %q", again.SavedAs, st.SavedAs)
	}

	a.DismissLoadTest()
	if idle := a.PollLoadTest(); idle.Running {
		t.Error("dismissed run should poll as idle")
	}
}

func TestResultsListAndDelete(t *testing.T) {
	a := testApp(t)
	if results, err := a.ListResults(); err != nil || len(results) != 0 {
		t.Fatalf("empty store: %v, %v", results, err)
	}

	older := time.Now().Add(-time.Hour)
	for i, at := range []time.Time{older, time.Now()} {
		s := loadtest.Summary{
			Profile: "quick", Method: "GET", URL: "https://x.test",
			StartedAt: at, Completed: 100 + i, OK: 100 + i,
			LatencyP99: loadtest.Duration(time.Duration(5+i) * time.Millisecond),
		}
		if _, err := a.resultStore.Save(s); err != nil {
			t.Fatal(err)
		}
	}

	results, err := a.ListResults()
	if err != nil || len(results) != 2 {
		t.Fatalf("ListResults = %d results, %v", len(results), err)
	}
	if results[0].Completed != 101 {
		t.Error("results should be newest first")
	}
	if results[0].P99MS != 6 || results[0].Text == "" || results[0].File == "" {
		t.Errorf("DTO not flattened: %+v", results[0])
	}

	if err := a.DeleteResult(results[1].File); err != nil {
		t.Fatal(err)
	}
	if results, _ := a.ListResults(); len(results) != 1 {
		t.Errorf("delete left %d results", len(results))
	}
	if err := a.DeleteResult("../escape.json"); err == nil {
		t.Error("path traversal in result name must be rejected")
	}
}

func TestUseEnvironmentOffAndUnknown(t *testing.T) {
	a := testApp(t)
	if _, err := a.UseEnvironment("nosuch"); err == nil {
		t.Error("unknown environment should error")
	}
	if err := a.envStore.Save("dev", map[string]string{"a": "1"}); err != nil {
		t.Fatal(err)
	}
	st, err := a.UseEnvironment("dev")
	if err != nil || st.Active != "dev" {
		t.Fatalf("activate: %+v, %v", st, err)
	}
	st, err = a.UseEnvironment("")
	if err != nil || st.Active != "" {
		t.Errorf("deactivate: %+v, %v", st, err)
	}
	if a.envVars != nil {
		t.Error("deactivation should drop the loaded vars")
	}
}
