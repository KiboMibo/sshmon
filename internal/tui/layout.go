package tui

const (
	minimumWidth  = 60
	minimumHeight = 16
	wideWidth     = 100
)

type layoutState struct {
	width    int
	height   int
	wide     bool
	tooSmall bool
}

func newLayout(width, height int) layoutState {
	return layoutState{
		width:    width,
		height:   height,
		wide:     width >= wideWidth && height >= minimumHeight,
		tooSmall: width < minimumWidth || height < minimumHeight,
	}
}
