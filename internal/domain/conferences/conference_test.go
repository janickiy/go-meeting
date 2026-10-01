package conferences

import "testing"

// TestLifecycleAllowsOnlyForwardTransitions проверяет сценарий «жизненный цикл разрешает только Forward переходы», фиксируя ошибки поведения как регрессию.
//
// @parameters:
//   - t (*testing.T): контекст теста: сообщает об ошибках, управляет вспомогательными проверками и очисткой.
func TestLifecycleAllowsOnlyForwardTransitions(t *testing.T) {
	statuses := []Status{Created, Active, Finished, Cancelled, "unknown"}
	for _, from := range statuses {
		for _, to := range statuses {
			want := (from == Created && (to == Active || to == Cancelled)) || (from == Active && to == Finished)
			if got := CanTransition(from, to); got != want {
				t.Errorf("CanTransition(%s, %s) = %v, want %v", from, to, got, want)
			}
		}
	}
}
