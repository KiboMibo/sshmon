package collect

import "github.com/kibomibo/sshmon/internal/sshx"

// Sentinel-ошибки, которые UI матчит через errors.Is. Живут в collect, чтобы
// слой представления не импортировал транспорт sshx напрямую.
//
// Выбраны алиасы, а не собственные значения с обёрткой: обёртка (`fmt.Errorf`
// с %w на каждом возврате из sshx или свой метод Is) дала бы то же поведение
// errors.Is, но потребовала бы перехватывать каждую ошибку транспорта на
// границе пакета. Алиас решает задачу одной строкой и работает в обе стороны
// по определению — значение то же самое. Цена: collect не может отличить
// «свою» ошибку от «чужой», но для sentinel'ов это и не требуется.
var (
	ErrPassphraseRequired = sshx.ErrPassphraseRequired
	ErrInvalidPassphrase  = sshx.ErrInvalidPassphrase
)
