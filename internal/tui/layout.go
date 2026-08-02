package tui

const (
	minimumWidth  = 60
	minimumHeight = 16
	// twoColumnWidth — порог, с которого средний ряд экрана сервера и список
	// хостов раскладываются в две колонки. Ниже него состав экрана тот же,
	// но колонки схлопываются в одну: отдельной «узкой» раскладки больше нет.
	twoColumnWidth = 100
)

type layoutState struct {
	width    int
	height   int
	tooSmall bool
}

// newLayout принимает размер терминала как есть: внешней рамки у экрана нет,
// поэтому кадр занимает весь терминал и вычитать под рамку нечего.
func newLayout(width, height int) layoutState {
	return layoutState{
		width:    width,
		height:   height,
		tooSmall: width < minimumWidth || height < minimumHeight,
	}
}

// twoColumn отвечает на вопрос «помещаются ли две колонки», а не выбирает
// раскладку целиком: экран один, он деградирует по ширине и высоте по месту.
func (l layoutState) twoColumn() bool {
	return l.width >= twoColumnWidth
}
