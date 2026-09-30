package conferences

import "testing"

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
