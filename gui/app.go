package main

// app.go — the Wails binding layer: a thin App over the same internal
// packages the TUI uses (collections, vars, build, httpx). Every method here
// is callable from the TypeScript front-end. No feature logic lives here —
// only DTO shaping — so a request sends identically in both front-ends.

import (
	"context"
	"time"

	"github.com/tabularasa/volley/internal/build"
	"github.com/tabularasa/volley/internal/codegen"
	"github.com/tabularasa/volley/internal/collections"
	"github.com/tabularasa/volley/internal/curl"
	"github.com/tabularasa/volley/internal/httpx"
	"github.com/tabularasa/volley/internal/loadtest"
	"github.com/tabularasa/volley/internal/model"
	"github.com/tabularasa/volley/internal/vars"
)

// App is the bound object.
type App struct {
	ctx context.Context

	store       collections.Store
	envStore    vars.EnvStore
	loadStore   loadtest.Store
	resultStore loadtest.ResultStore

	session vars.Store        // :set-equivalent overrides, in-memory
	envName string            // active environment, "" for none
	envVars map[string]string // active environment's variables

	load loadState // the in-flight load test (see loadtest.go)
}

func newApp(store collections.Store, envStore vars.EnvStore, loadStore loadtest.Store, resultStore loadtest.ResultStore) *App {
	return &App{store: store, envStore: envStore, loadStore: loadStore, resultStore: resultStore, session: vars.New()}
}

// startup is Wails' OnStartup hook.
func (a *App) startup(ctx context.Context) { a.ctx = ctx }

func (a *App) resolver() vars.Layered {
	return vars.Layered{a.session, a.envVars}
}

// --- DTOs (JSON-shaped for the front-end) ---

type HeaderDTO struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	Enabled bool   `json:"enabled"`
}

type KVDTO struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Enabled bool   `json:"enabled"`
}

type AuthDTO struct {
	Type     string `json:"type"` // "" | bearer | basic | apikey
	Token    string `json:"token,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Key      string `json:"key,omitempty"`
	Value    string `json:"value,omitempty"`
	InQuery  bool   `json:"inQuery,omitempty"`
}

type RequestDTO struct {
	Method    string      `json:"method"`
	URL       string      `json:"url"`
	Headers   []HeaderDTO `json:"headers"`
	Query     []KVDTO     `json:"query"`
	Body      string      `json:"body"`
	Auth      AuthDTO     `json:"auth"`
	TimeoutMS int         `json:"timeoutMs"` // 0 = engine default
}

type ResponseDTO struct {
	Status     string      `json:"status"`
	StatusCode int         `json:"statusCode"`
	Proto      string      `json:"proto"`
	Headers    []HeaderDTO `json:"headers"`
	Body       string      `json:"body"`
	DurationMS int64       `json:"durationMs"`
	Size       int64       `json:"size"`
	Truncated  bool        `json:"truncated"`
	Error      string      `json:"error,omitempty"`
	FinalURL   string      `json:"finalUrl"` // URL after {{vars}} and query folding
}

type TreeItemDTO struct {
	Name   string `json:"name"` // slash-separated, e.g. "auth/login"
	IsDir  bool   `json:"isDir"`
	Method string `json:"method,omitempty"`
}

func toRequest(d RequestDTO) model.Request {
	req := model.Request{
		Method:  d.Method,
		URL:     d.URL,
		Body:    d.Body,
		Timeout: time.Duration(d.TimeoutMS) * time.Millisecond,
		Auth: model.Auth{
			Type: d.Auth.Type, Token: d.Auth.Token,
			Username: d.Auth.Username, Password: d.Auth.Password,
			Key: d.Auth.Key, Value: d.Auth.Value, InQuery: d.Auth.InQuery,
		},
	}
	for _, h := range d.Headers {
		req.Headers = append(req.Headers, model.Header(h))
	}
	for _, kv := range d.Query {
		req.Query = append(req.Query, model.KV(kv))
	}
	return req
}

func fromRequest(req model.Request) RequestDTO {
	d := RequestDTO{
		Method:    req.Method,
		URL:       req.URL,
		Body:      req.Body,
		Headers:   []HeaderDTO{},
		Query:     []KVDTO{},
		TimeoutMS: int(req.Timeout / time.Millisecond),
		Auth: AuthDTO{
			Type: req.Auth.Type, Token: req.Auth.Token,
			Username: req.Auth.Username, Password: req.Auth.Password,
			Key: req.Auth.Key, Value: req.Auth.Value, InQuery: req.Auth.InQuery,
		},
	}
	for _, h := range req.Headers {
		d.Headers = append(d.Headers, HeaderDTO(h))
	}
	for _, kv := range req.Query {
		d.Query = append(d.Query, KVDTO(kv))
	}
	return d
}

// --- requests: send ---

// Send builds the final wire request ({{vars}} → auth → query folding) and
// executes it. Transport errors come back inside the DTO, never as a Wails
// error, so the front-end always has timing to render.
func (a *App) Send(d RequestDTO) ResponseDTO {
	built := build.Final(toRequest(d), a.resolver())
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	resp := httpx.Do(ctx, built)

	out := ResponseDTO{
		Status:     resp.Status,
		StatusCode: resp.StatusCode,
		Proto:      resp.Proto,
		Headers:    []HeaderDTO{},
		Body:       string(resp.Body),
		DurationMS: resp.Duration.Milliseconds(),
		Size:       resp.Size,
		Truncated:  resp.Truncated,
		FinalURL:   built.URL,
	}
	for _, h := range resp.Headers {
		out.Headers = append(out.Headers, HeaderDTO(h))
	}
	if resp.Err != nil {
		out.Error = resp.Err.Error()
	}
	return out
}

// BuiltURL previews where the request would actually go — {{vars}} expanded
// and query folded — for the load-test confirm step and the URL bar hint.
func (a *App) BuiltURL(d RequestDTO) string {
	return build.Final(toRequest(d), a.resolver()).URL
}

// Unresolved reports the {{placeholders}} the resolver cannot satisfy, so the
// front-end can warn before sending (the TUI blocks the send on the same
// check).
func (a *App) Unresolved(d RequestDTO) []string {
	built := build.Final(toRequest(d), a.resolver())
	names := vars.Unresolved(built)
	if names == nil {
		names = []string{}
	}
	return names
}

// --- requests: collection store ---

func (a *App) ListRequests() ([]TreeItemDTO, error) {
	items, err := a.store.List()
	if err != nil {
		return nil, err
	}
	out := []TreeItemDTO{}
	for _, it := range items {
		out = append(out, TreeItemDTO{Name: it.Name, IsDir: it.IsDir, Method: it.Method})
	}
	return out, nil
}

func (a *App) LoadRequest(name string) (RequestDTO, error) {
	req, err := a.store.Load(name)
	if err != nil {
		return RequestDTO{}, err
	}
	return fromRequest(req), nil
}

func (a *App) SaveRequest(name string, d RequestDTO) error {
	return a.store.Save(name, toRequest(d))
}

func (a *App) DeleteRequest(name string) error {
	return a.store.Delete(name)
}

func (a *App) RenameRequest(oldName, newName string) error {
	return a.store.Rename(oldName, newName)
}

func (a *App) CopyRequest(oldName, newName string) error {
	return a.store.Copy(oldName, newName)
}

func (a *App) CreateGroup(name string) error {
	return a.store.CreateGroup(name)
}

func (a *App) DeleteGroup(name string) error {
	return a.store.DeleteGroup(name)
}

func (a *App) RenameGroup(oldName, newName string) error {
	return a.store.RenameGroup(oldName, newName)
}

// --- curl import / export ---

type CurlImportDTO struct {
	Request  RequestDTO `json:"request"`
	Warnings []string   `json:"warnings"`
}

// ImportCurl parses a pasted curl command into an editable request,
// reporting the flags it had to skip — the GUI's :import curl.
func (a *App) ImportCurl(cmd string) (CurlImportDTO, error) {
	req, warns, err := curl.Parse(cmd)
	if err != nil {
		return CurlImportDTO{}, err
	}
	if warns == nil {
		warns = []string{}
	}
	return CurlImportDTO{Request: fromRequest(req), Warnings: warns}, nil
}

// ExportCurl renders the request as a curl command. Like the TUI's
// :copy curl it exports the BUILT request — {{vars}} expanded, auth
// materialized, query folded — so the command works when pasted anywhere.
func (a *App) ExportCurl(d RequestDTO) string {
	return curl.Format(build.Final(toRequest(d), a.resolver()))
}

// GenerateCode renders the BUILT request in the named CLI dialect
// (curl / wget / httpie) — the Bruno-style code button next to Send.
func (a *App) GenerateCode(format string, d RequestDTO) (string, error) {
	return codegen.Generate(format, build.Final(toRequest(d), a.resolver()))
}

// --- session variables ---

// SessionVars returns the in-memory :set-equivalent overrides.
func (a *App) SessionVars() map[string]string {
	out := map[string]string{}
	for k, v := range a.session {
		out[k] = v
	}
	return out
}

// SetSessionVar defines (or, with an empty value, removes) a session
// override.
func (a *App) SetSessionVar(name, value string) {
	if value == "" {
		delete(a.session, name)
		return
	}
	a.session.Set(name, value)
}

// --- environments ---

type EnvStateDTO struct {
	Active string   `json:"active"` // "" = none
	Names  []string `json:"names"`
}

func (a *App) envState() (EnvStateDTO, error) {
	names, err := a.envStore.List()
	if err != nil {
		return EnvStateDTO{}, err
	}
	if names == nil {
		names = []string{}
	}
	return EnvStateDTO{Active: a.envName, Names: names}, nil
}

func (a *App) Environments() (EnvStateDTO, error) { return a.envState() }

// UseEnvironment activates a stored environment; the empty string
// deactivates.
func (a *App) UseEnvironment(name string) (EnvStateDTO, error) {
	if name == "" {
		a.envName, a.envVars = "", nil
		return a.envState()
	}
	vals, err := a.envStore.Load(name)
	if err != nil {
		return EnvStateDTO{}, err
	}
	a.envName, a.envVars = name, vals
	return a.envState()
}

// GetEnvironment returns a stored environment's variables for editing.
func (a *App) GetEnvironment(name string) (map[string]string, error) {
	return a.envStore.Load(name)
}

// SaveEnvironment writes an environment and — matching the TUI's :envedit
// round-trip — activates it, since editing one almost always means you're
// about to use it.
func (a *App) SaveEnvironment(name string, vals map[string]string) (EnvStateDTO, error) {
	if err := a.envStore.Save(name, vals); err != nil {
		return EnvStateDTO{}, err
	}
	a.envName, a.envVars = name, vals
	return a.envState()
}

// DeleteEnvironment removes a stored environment, deactivating it first if
// it was active.
func (a *App) DeleteEnvironment(name string) (EnvStateDTO, error) {
	if err := a.envStore.Delete(name); err != nil {
		return EnvStateDTO{}, err
	}
	if a.envName == name {
		a.envName, a.envVars = "", nil
	}
	return a.envState()
}
