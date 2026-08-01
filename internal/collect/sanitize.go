package collect

import "strings"

// SanitizeLine вычищает одну строку текста, пришедшего с удалённого хоста.
//
// Содержимое лога подконтрольно чужому процессу целиком, и escape-последователь-
// ности в нём — не оформление, а команды терминалу оператора: OSC 52 молча
// переписывает буфер обмена, OSC 0/2 подделывает заголовок окна, CSI двигает
// курсор и ломает раскладку кадра, а запросы вида «ответь мне» заставляют часть
// эмуляторов напечатать ответ в stdin приложения. Поэтому вырезаем всё
// управляющее, а не пытаемся «безопасно» пропустить цвета.
//
// Санитизация стоит на входе — в момент, когда строка становится данными
// приложения (буфер логов, разобранные структуры, текст ошибки), а не на
// выводе. Причина: тот же текст уходит не только в TUI, но и наружу — MCP-сервер
// отдаёт хвост лога другим клиентам, чей терминал мы не рисуем. Чистить в каждом
// потребителе означало бы однажды забыть про одного из них. Собственная
// стилизация приложения (подсветка фильтра, цвета уровней, маркер выделения)
// накладывается уже поверх — она добавляется на этапе рендера, то есть после.
func SanitizeLine(line string) string {
	if strings.IndexFunc(line, unsafeControl) < 0 {
		return line
	}
	runes := []rune(line)
	var b strings.Builder
	b.Grow(len(line))
	for index := 0; index < len(runes); {
		switch r := runes[index]; {
		case r == 0x1b:
			index = escapeSequenceEnd(runes, index)
		case r == '\t', r == '\n', r == '\r':
			// Пробельные становятся пробелом, а не выбрасываются: иначе соседние
			// поля строки слиплись бы в одно слово. Перевода строки в построчном
			// буфере быть не должно, но пришедший внутри строки «\n» разъехался бы
			// с нумерацией строк экрана — он тоже пробел.
			b.WriteByte(' ')
			index++
		case unsafeControl(r):
			index++
		default:
			b.WriteRune(r)
			index++
		}
	}
	return b.String()
}

// unsafeControl — управляющий символ, которому нечего делать в выводе: C0 без
// пробельных, DEL и C1 (0x9b — тот же CSI, только одним символом).
func unsafeControl(r rune) bool {
	if r == '\t' || r == '\n' || r == '\r' {
		return true
	}
	return r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f)
}

// escapeSequenceEnd возвращает индекс за концом escape-последовательности,
// начинающейся на runes[start] == ESC. Незакрытая последовательность съедает
// остаток строки — это и есть верное поведение: терминал поступил бы так же.
func escapeSequenceEnd(runes []rune, start int) int {
	index := start + 1
	if index >= len(runes) {
		return len(runes)
	}
	switch runes[index] {
	case '[': // CSI: параметры, затем финальный байт 0x40–0x7E
		for index++; index < len(runes); index++ {
			if runes[index] >= 0x40 && runes[index] <= 0x7e {
				return index + 1
			}
		}
	case ']', 'P', 'X', '^', '_': // OSC и прочие строковые команды: до BEL или ST
		for index++; index < len(runes); index++ {
			if runes[index] == 0x07 {
				return index + 1
			}
			if runes[index] == 0x1b && index+1 < len(runes) && runes[index+1] == '\\' {
				return index + 2
			}
		}
	default: // двухсимвольная последовательность
		return index + 1
	}
	return len(runes)
}

// sanitizeFields чистит поля разобранной строки на месте. Нужен там, где строку
// нельзя чистить целиком до разбора: `docker ps --format` разделяет колонки
// табуляцией, а SanitizeLine превращает её в пробел.
func sanitizeFields(fields []string) []string {
	for i, field := range fields {
		fields[i] = SanitizeLine(field)
	}
	return fields
}
