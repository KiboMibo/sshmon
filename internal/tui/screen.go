package tui

// screen renders one screen's own state from an explicit screenContext instead
// of reaching into the whole Model. That one-way seam — screens read context,
// never mutate Model — is the boundary the internal/tui god-package split is
// built on (ASSESSMENT.md #1). processes is the pilot; other screens adopt the
// same shape before each moves to its own package.
type screen interface {
	view(ctx screenContext) string
}

// screenContext is the read-only slice of root state a screen needs to render.
type screenContext struct {
	serverName string
}

func (m Model) screenContext() screenContext {
	return screenContext{serverName: m.selectedName()}
}
