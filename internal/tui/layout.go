package tui

const (
	minimumWidth  = 60
	minimumHeight = 16
	wideWidth     = 100
	frameOverhead = 2
)

type layoutState struct {
	width    int
	height   int
	wide     bool
	tooSmall bool
}

func newLayout(width, height int) layoutState {
	return layoutState{
		width:    width - frameOverhead,
		height:   height - frameOverhead,
		wide:     width >= wideWidth && height >= minimumHeight,
		tooSmall: width < minimumWidth || height < minimumHeight,
	}
}
