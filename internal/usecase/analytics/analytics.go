// Package analytics строит ограниченные агрегаты присутствия и очереди фонового пересчёта.
package analytics

import (
	"context"
	domain "github.com/janickiy/go-recorder/internal/domain/analytics"
	"github.com/janickiy/go-recorder/internal/domain/jobs"
	"github.com/janickiy/go-recorder/internal/operations"
	"sort"
	"time"
)

// Repository предоставляет интервалы и атомарную публикацию с job lease fencing.
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
// @args cid — встреча; start,end — её границы; intervals — bounded интервалы присутствия.
// @return агрегаты с хронологией, без сортировки участников по времени речи.
func Aggregate(cid string, start, end time.Time, intervals []domain.Interval) domain.Conference {
	result := domain.Conference{ConferenceID: cid, Enabled: true, ApproximateSpeaking: true, Timeline: []domain.Point{}, Participants: []domain.Participant{}, DurationMS: max(int64(0), end.Sub(start).Milliseconds())}
	groups := map[string][]domain.Interval{}
	for _, v := range intervals {
		if v.Start.Before(start) {
			v.Start = start
		}
		if v.End.After(end) {
			v.End = end
		}
		if v.End.After(v.Start) {
			groups[v.ParticipantID] = append(groups[v.ParticipantID], v)
		}
	}
	changes := map[int64]int{}
	ids := make([]string, 0, len(groups))
	for id := range groups {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		list := groups[id]
		sort.Slice(list, func(i, j int) bool { return list[i].Start.Before(list[j].Start) })
		var duration int64
		merged := []domain.Interval{}
		for _, v := range list {
			n := len(merged)
			if n > 0 && !v.Start.After(merged[n-1].End) {
				if v.End.After(merged[n-1].End) {
					merged[n-1].End = v.End
				}
			} else {
				merged = append(merged, v)
			}
		}
		for _, v := range merged {
			duration += v.End.Sub(v.Start).Milliseconds()
			changes[v.Start.Sub(start).Milliseconds()]++
			changes[v.End.Sub(start).Milliseconds()]--
		}
		result.Participants = append(result.Participants, domain.Participant{ParticipantID: id, ParticipationMS: duration})
	}
	result.ParticipantCount = len(result.Participants)
	times := make([]int64, 0, len(changes))
	for at := range changes {
		times = append(times, at)
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	bucket := max(int64(1), (result.DurationMS+598)/599)
	count := 0
	for _, at := range times {
		count += changes[at]
		point := domain.Point{AtMS: at, Count: max(0, count)}
		n := len(result.Timeline)
		if n > 0 && result.Timeline[n-1].AtMS/bucket == at/bucket {
			result.Timeline[n-1] = point
		} else {
			result.Timeline = append(result.Timeline, point)
		}
	}
	return result
}

// Handle пересчитывает один conference aggregate в отдельном product-worker.
// @args ctx — deadline; job — постоянное задание.
// @return ошибка SQL/fencing или осознанный skip.
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
