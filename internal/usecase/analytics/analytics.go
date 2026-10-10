// Пакет analytics строит ограниченные агрегаты присутствия и очередь фонового пересчёта.
package analytics

import (
	"context"
	domain "github.com/janickiy/go-recorder/internal/domain/analytics"
	"github.com/janickiy/go-recorder/internal/domain/jobs"
	"github.com/janickiy/go-recorder/internal/operations"
	"sort"
	"time"
)

// Repository предоставляет интервалы и атомарную публикацию с проверкой действующей аренды задания.
type Repository interface {
	Source(context.Context, jobs.Job) (time.Time, time.Time, []domain.Interval, error)
	Save(context.Context, jobs.Job, domain.Conference) error
	Read(context.Context, string, string) (domain.Conference, error)
	Tick(context.Context) error
}

// Service не зависит от STT; отказ распознавания не отменяет технические показатели встречи.
type Service struct {
	Repo    Repository
	Enabled bool
}

// Aggregate объединяет перекрывающиеся вкладки одного участника и ограничивает timeline 600 точками.
// @args conferenceID — встреча; start,end — её границы; intervals — ограниченный набор интервалов присутствия.
// @return агрегаты с хронологией, без сортировки участников по времени речи.
func Aggregate(conferenceID string, start, end time.Time, intervals []domain.Interval) domain.Conference {
	result := domain.Conference{ConferenceID: conferenceID, Enabled: true, ApproximateSpeaking: true, Timeline: []domain.Point{}, Participants: []domain.Participant{}, DurationMS: max(int64(0), end.Sub(start).Milliseconds())}
	intervalsByParticipant := map[string][]domain.Interval{}
	for _, interval := range intervals {
		if interval.Start.Before(start) {
			interval.Start = start
		}
		if interval.End.After(end) {
			interval.End = end
		}
		if interval.End.After(interval.Start) {
			intervalsByParticipant[interval.ParticipantID] = append(intervalsByParticipant[interval.ParticipantID], interval)
		}
	}
	changes := map[int64]int{}
	participantIDs := make([]string, 0, len(intervalsByParticipant))
	for participantID := range intervalsByParticipant {
		participantIDs = append(participantIDs, participantID)
	}
	sort.Strings(participantIDs)
	for _, participantID := range participantIDs {
		participantIntervals := intervalsByParticipant[participantID]
		sort.Slice(participantIntervals, func(i, j int) bool { return participantIntervals[i].Start.Before(participantIntervals[j].Start) })
		var participationMS int64
		merged := []domain.Interval{}
		for _, interval := range participantIntervals {
			n := len(merged)
			if n == 0 || interval.Start.After(merged[n-1].End) {
				merged = append(merged, interval)
				continue
			}
			if interval.End.After(merged[n-1].End) {
				merged[n-1].End = interval.End
			}
		}
		for _, interval := range merged {
			participationMS += interval.End.Sub(interval.Start).Milliseconds()
			changes[interval.Start.Sub(start).Milliseconds()]++
			changes[interval.End.Sub(start).Milliseconds()]--
		}
		result.Participants = append(result.Participants, domain.Participant{ParticipantID: participantID, ParticipationMS: participationMS})
	}
	result.ParticipantCount = len(result.Participants)
	changeTimes := make([]int64, 0, len(changes))
	for at := range changes {
		changeTimes = append(changeTimes, at)
	}
	sort.Slice(changeTimes, func(i, j int) bool { return changeTimes[i] < changeTimes[j] })
	bucketMS := max(int64(1), (result.DurationMS+598)/599)
	count := 0
	for _, at := range changeTimes {
		count += changes[at]
		point := domain.Point{AtMS: at, Count: max(0, count)}
		n := len(result.Timeline)
		if n > 0 && result.Timeline[n-1].AtMS/bucketMS == at/bucketMS {
			result.Timeline[n-1] = point
		} else {
			result.Timeline = append(result.Timeline, point)
		}
	}
	return result
}

// Handle пересчитывает агрегат одной конференции в отдельном product-worker.
// @args ctx — срок выполнения; job — постоянное задание.
// @return ошибка SQL, потеря актуальности аренды либо осознанный пропуск обработки.
func (s *Service) Handle(ctx context.Context, job jobs.Job) error {
	if !s.Enabled {
		return jobs.ErrSkip
	}
	started := time.Now()
	defer func() { operations.Observe("analytics", time.Since(started).Seconds()) }()
	start, end, rows, err := s.Repo.Source(ctx, job)
	if err != nil {
		return err
	}
	return s.Repo.Save(ctx, job, Aggregate(job.ConferenceID, start, end, rows))
}
