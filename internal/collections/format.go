package collections

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/tabularasa/volley/internal/model"
)

// schemaVersion is the on-disk format version for a saved request. Bump it
// whenever the persisted shape changes in a way that needs migrating on Load.
const schemaVersion = 1

// storedRequest is the on-disk representation of a model.Request. It is kept
// separate from the domain type so the JSON schema — field names, the version
// tag, and the human-readable timeout — can evolve without leaking storage
// concerns into the rest of the app.
type storedRequest struct {
	SchemaVersion int            `json:"schemaVersion"`
	Method        string         `json:"method"`
	URL           string         `json:"url"`
	Headers       []storedHeader `json:"headers,omitempty"`
	Query         []storedKV     `json:"query,omitempty"`
	Body          string         `json:"body,omitempty"`
	Timeout       duration       `json:"timeout,omitempty"`
}

type storedHeader struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	Enabled bool   `json:"enabled"`
}

type storedKV struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Enabled bool   `json:"enabled"`
}

// duration serializes as a Go duration string ("10s") but also accepts a bare
// number of nanoseconds, so requests written before the string format (v0,
// which had no schemaVersion) still load.
type duration time.Duration

func (d duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

func (d *duration) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		*d = 0
		return nil
	}
	if b[0] == '"' { // current format: a duration string like "10s"
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		if s == "" {
			*d = 0
			return nil
		}
		parsed, err := time.ParseDuration(s)
		if err != nil {
			return err
		}
		*d = duration(parsed)
		return nil
	}
	// Legacy format: raw nanoseconds.
	var n int64
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*d = duration(n)
	return nil
}

// toStored converts a domain request into its on-disk form, stamping the
// current schema version.
func toStored(req model.Request) storedRequest {
	sr := storedRequest{
		SchemaVersion: schemaVersion,
		Method:        req.Method,
		URL:           req.URL,
		Body:          req.Body,
		Timeout:       duration(req.Timeout),
	}
	for _, h := range req.Headers {
		sr.Headers = append(sr.Headers, storedHeader{Name: h.Name, Value: h.Value, Enabled: h.Enabled})
	}
	for _, q := range req.Query {
		sr.Query = append(sr.Query, storedKV{Key: q.Key, Value: q.Value, Enabled: q.Enabled})
	}
	return sr
}

// toModel converts an on-disk request back into the domain type, applying the
// same GET fallback the app relies on for older/hand-edited files.
func (sr storedRequest) toModel() model.Request {
	req := model.Request{
		Method:  sr.Method,
		URL:     sr.URL,
		Body:    sr.Body,
		Timeout: time.Duration(sr.Timeout),
	}
	for _, h := range sr.Headers {
		req.Headers = append(req.Headers, model.Header{Name: h.Name, Value: h.Value, Enabled: h.Enabled})
	}
	for _, q := range sr.Query {
		req.Query = append(req.Query, model.KV{Key: q.Key, Value: q.Value, Enabled: q.Enabled})
	}
	if req.Method == "" {
		req.Method = "GET"
	}
	return req
}

// writeFileAtomic writes data to path by way of a temp file in the same
// directory, fsync, and a rename — so a crash or full disk mid-write can never
// leave a saved request truncated or half-written.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".volley-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	// Best-effort cleanup if we bail out before the rename lands.
	defer func() {
		if tmpName != "" {
			os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	tmpName = "" // renamed into place; nothing to clean up
	return nil
}
