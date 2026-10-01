package notificationsapp

import "sync"

// syncCounts синхронизирует счётчики SSE-подключений для ограничения общего и пользовательского числа потоков.
//   - mu: блокировка согласованного доступа к разделяемому состоянию.
//   - values: индекс значений values для поиска и согласования состояния.
type syncCounts struct {
	mu     sync.Mutex
	values map[string]int
}

// acquire занимает слот ограниченного ресурса до запуска операции.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @parameters:
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//
// Результат:
//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
func (s *syncCounts) acquire(userID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.values[userID] >= 4 {
		return false
	}
	s.values[userID]++
	return true
}

// release освобождает ранее занятый слот ресурса.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @parameters:
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
func (s *syncCounts) release(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[userID]--
	if s.values[userID] <= 0 {
		delete(s.values, userID)
	}
}
