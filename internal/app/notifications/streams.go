package notificationsapp

import "sync"

type syncCounts struct {
	mu     sync.Mutex
	values map[string]int
}

func (s *syncCounts) acquire(userID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.values[userID] >= 4 {
		return false
	}
	s.values[userID]++
	return true
}
func (s *syncCounts) release(userID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[userID]--
	if s.values[userID] <= 0 {
		delete(s.values, userID)
	}
}
