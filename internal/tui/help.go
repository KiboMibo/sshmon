package tui

import "strings"

type helpRow struct {
	keys string
	desc string
}

type helpSection struct {
	title string
	rows  []helpRow
}

func helpTitle(kind screenKind) string {
	switch kind {
	case screenFleet:
		return "КЛАВИШИ · список хостов"
	case screenDashboard:
		return "КЛАВИШИ · сервер"
	case screenProcesses:
		return "КЛАВИШИ · процессы"
	case screenPorts:
		return "КЛАВИШИ · порты"
	case screenContainers:
		return "КЛАВИШИ · контейнеры"
	case screenHistory:
		return "КЛАВИШИ · история"
	case screenLogs:
		return "КЛАВИШИ · логи"
	default:
		return "КЛАВИШИ"
	}
}

func helpSections(kind screenKind) []helpSection {
	common := helpSection{title: "прочее", rows: []helpRow{
		{"c", "чат с ассистентом"},
		{":", "палитра команд"},
		{"?", "эта справка"},
	}}
	switch kind {
	case screenFleet:
		return []helpSection{
			{title: "движение", rows: []helpRow{
				{"↑↓ j k", "по строкам"},
				{"pgup pgdown", "страницами"},
				{"← →", "свернуть · раскрыть детали"},
				{"tab a", "следующая группа · все хосты"},
			}},
			{title: "найти", rows: []helpRow{
				{"/", "поиск по имени, адресу и группе"},
				{"f", "только проблемные"},
				{"esc", "сбросить фильтры"},
			}},
			{title: "сервер", rows: []helpRow{
				{"enter", "полный экран сервера"},
				{"l", "ящик логов над списком"},
				{"p o d", "процессы · порты · контейнеры (в деталях)"},
				{"x", "ssh в терминале (в деталях)"},
			}},
			{title: "прочее", rows: []helpRow{
				{"v", "боковая панель"},
				{"c :", "чат · палитра команд"},
				{"? q", "эта справка · выход"},
			}},
		}
	case screenDashboard:
		return []helpSection{
			{title: "движение", rows: []helpRow{
				{"tab shift+tab", "фокус панели"},
				{"j k", "прокрутка внутри панели"},
			}},
			{title: "разделы", rows: []helpRow{
				{"p", "процессы"},
				{"o d", "порты · контейнеры"},
				{"ctrl+l", "логи на весь экран"},
				{"ctrl+h", "история метрик"},
			}},
			{title: "сервисы", rows: []helpRow{
				{"f", "фильтр юнитов"},
				{"enter", "journal выбранного юнита"},
				{"x", "вернуть системный лог"},
				{"r", "переподключить сервер"},
			}},
			{title: "прочее", rows: append(common.rows, helpRow{"esc", "назад к списку"})},
		}
	case screenLogs:
		return []helpSection{
			{title: "фильтры", rows: []helpRow{
				{"/", "фильтр по подстроке"},
				{"w", "уровень: all · info · warn · error"},
				{"← → x", "источник"},
			}},
			{title: "поток", rows: []helpRow{
				{"space", "пауза хвоста"},
				{"r", "переподключить"},
			}},
			{title: "движение", rows: []helpRow{
				{"↑↓ pgup pgdown", "прокрутка"},
				{"home end", "в начало · в конец"},
			}},
			{title: "прочее", rows: append(common.rows, helpRow{"esc", "назад"})},
		}
	case screenHistory:
		return []helpSection{
			{title: "данные", rows: []helpRow{
				{"1-5", "диапазон времени"},
				{"j k", "метрика"},
				{"h l", "курсор по точкам"},
				{"r", "обновить"},
			}},
			{title: "прочее", rows: append(common.rows, helpRow{"esc", "назад"})},
		}
	default:
		return []helpSection{
			{title: "экран", rows: []helpRow{
				{"", "только чтение, обновляется сам"},
			}},
			{title: "прочее", rows: append(common.rows, helpRow{"esc", "назад"})},
		}
	}
}

func helpText(kind screenKind) string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(helpTitle(kind)))
	for _, section := range helpSections(kind) {
		b.WriteString("\n\n" + titleStyle.Render(section.title))
		for _, row := range section.rows {
			b.WriteString("\n  " + padCell(row.keys, 16) + dimStyle.Render(row.desc))
		}
	}
	b.WriteString("\n\n" + dimStyle.Render("esc / ? — закрыть · в статусбаре живут 3–4 самые частые клавиши"))
	return b.String()
}
