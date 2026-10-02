package ffmpeg

import "sync"

const maxStderrBytes = 64 * 1024

// logTail сохраняет только ограниченный хвост вывода внешнего процесса.
// @params
//   - mu: блокировка согласованного доступа к разделяемому состоянию.
//   - data: полезная нагрузка события или байты обрабатываемого содержимого.
type logTail struct {
	mu   sync.Mutex
	data []byte
}

// Write принимает байты вывода в ограниченный буфер и соблюдает контракт io.Writer.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - p ([]byte): байты, переданные по контракту io.Writer.
//
// @return:
//   - результат 1 (int): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (b *logTail) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	if n >= maxStderrBytes {
		b.data = append(b.data[:0], p[n-maxStderrBytes:]...)
		return n, nil
	}
	if overflow := len(b.data) + n - maxStderrBytes; overflow > 0 {
		b.data = b.data[:copy(b.data, b.data[overflow:])]
	}
	b.data = append(b.data, p...)
	return n, nil
}

// String возвращает строковое представление накопленного значения или ограниченного диагностического вывода.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @return:
//   - результат 1 (string): значение, подготовленное операцией для вызывающей стороны.
func (b *logTail) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.data)
}
