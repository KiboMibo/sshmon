package tui

import "strings"

func sparkline(values []float64, width int) string {
	if width <= 0 {
		return ""
	}
	if len(values) == 0 {
		return strings.Repeat("─", width)
	}
	const bars = "▁▂▃▄▅▆▇█"
	runes := []rune(bars)
	result := make([]rune, width)
	for i := range result {
		index := i * len(values) / width
		value := values[index]
		if value < 0 {
			value = 0
		}
		if value > 100 {
			value = 100
		}
		result[i] = runes[int(value*float64(len(runes)-1)/100)]
	}
	return string(result)
}
