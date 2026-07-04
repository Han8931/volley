package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tabularasa/volley/internal/httpx"
	"github.com/tabularasa/volley/internal/model"
)

// responseMsg carries a completed request back into the Update loop. seq ties
// the result to the send that started it, so a response arriving after the user
// cancelled it (or fired a newer request) is recognized as stale and dropped.
type responseMsg struct {
	seq  int
	resp model.Response
}

// sendCmd executes req off the UI goroutine and reports the result. The context
// is owned by the caller (Model.send), which cancels it to abort an in-flight
// request; the per-request timeout is applied inside httpx.Do.
func sendCmd(ctx context.Context, seq int, req model.Request) tea.Cmd {
	return func() tea.Msg {
		return responseMsg{seq: seq, resp: httpx.Do(ctx, req)}
	}
}
