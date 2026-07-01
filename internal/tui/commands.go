package tui

import (
	"context"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tabularasa/volley/internal/httpx"
	"github.com/tabularasa/volley/internal/model"
)

// responseMsg carries a completed request back into the Update loop.
type responseMsg struct{ resp model.Response }

// sendCmd executes req off the UI goroutine and reports the result.
func sendCmd(req model.Request) tea.Cmd {
	return func() tea.Msg {
		timeout := req.Timeout
		if timeout <= 0 {
			timeout = httpx.DefaultTimeout
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout+5*time.Second)
		defer cancel()
		return responseMsg{resp: httpx.Do(ctx, req)}
	}
}
