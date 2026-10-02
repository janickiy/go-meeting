package captions

import "sync"

// activityMeter считает объединение 20-ms аудио-интервалов одного участника, а не сумму устройств.
// Кольцевое окно 30 секунд и 500 участников ограничивают память; поздние старые кадры игнорируются.
type activityMeter struct {
	mu           sync.Mutex
	participants map[string]*activityWindow
}

// activityWindow хранит только бит наличия звука/активности, без PCM или текста.
type activityWindow struct {
	latest int64
	slots  [1500]activitySlot
}

// activitySlot защищает переиспользуемую ячейку кольца от повторного учёта одного момента.
type activitySlot struct {
	bucket             int64
	observed, speaking bool
}

// observe объединяет активность параллельных источников по абсолютному времени конференции.
// @args participant — серверный UUID; start,end — ms; active — результат детектора уровня.
// @return новые, ещё не учтённые миллисекунды речи и наблюдения.
func (m *activityMeter) observe(participant string, start, end int64, active bool) (speech, observed int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.participants == nil {
		m.participants = map[string]*activityWindow{}
	}
	w := m.participants[participant]
	if w == nil {
		if len(m.participants) >= 500 {
			return
		}
		w = &activityWindow{}
		m.participants[participant] = w
	}
	if start < 0 || end <= start || end-start > 1000 {
		return
	}
	for bucket := start / 20; bucket < end/20; bucket++ {
		if bucket < w.latest-1499 {
			continue
		}
		w.latest = max(w.latest, bucket)
		s := &w.slots[bucket%1500]
		if s.bucket != bucket {
			*s = activitySlot{bucket: bucket}
		}
		if !s.observed {
			s.observed = true
			observed += 20
		}
		if active && !s.speaking {
			s.speaking = true
			speech += 20
		}
	}
	return
}
