package postgres

import (
	"context"
	"time"

	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	"gorm.io/gorm"
)

func (r *ConferenceRepository) DecideAdmission(ctx context.Context, conferenceID, userID, participantID string, request conferences.AdmissionRequest) (conferences.Participant, error) {
	var target conferences.Participant
	if err := request.Validate(); err != nil {
		return target, err
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		c, err := findConference(tx, conferenceID, true)
		if err != nil {
			return err
		}
		if c.Status != conferences.Active {
			return apperrors.New(apperrors.ErrConflict, "conference is not active")
		}
		actor, err := findMembership(tx, conferenceID, userID)
		if err != nil {
			return membershipError(err)
		}
		if !actor.CanAdmit() {
			return apperrors.ErrForbidden
		}
		if err = tx.Where("id = ? AND conference_id = ?", participantID, conferenceID).Take(&target).Error; err != nil {
			return mapNotFound(err)
		}
		if target.Role == conferences.Owner || target.ID == actor.ID {
			return apperrors.ErrForbidden
		}
		wanted := conferences.AdmissionAdmitted
		if request.Decision == "reject" {
			wanted = conferences.AdmissionRejected
		}
		if target.AdmissionState == wanted {
			return nil
		}
		if target.AdmissionState != conferences.AdmissionWaiting || target.Status != conferences.Waiting {
			return apperrors.New(apperrors.ErrConflict, "participant is not waiting for admission")
		}
		now := time.Now().UTC()
		updates := map[string]any{"admission_state": wanted, "admission_decided_at": now, "admission_version": gorm.Expr("admission_version + 1"), "media_policy_version": gorm.Expr("media_policy_version + 1")}
		if wanted == conferences.AdmissionAdmitted {
			updates["status"], updates["joined_at"], updates["left_at"] = conferences.Joined, now, nil
		} else {
			updates["status"] = conferences.Rejected
			if target.JoinedAt != nil {
				updates["left_at"] = now
			}
		}
		if err = tx.Model(&target).Updates(updates).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", target.ID).Take(&target).Error
	})
	return target, err
}

func (r *ConferenceRepository) UpdateSchedule(ctx context.Context, conferenceID, userID string, request conferences.ScheduleRequest) (conferences.Conference, error) {
	var c conferences.Conference
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		c, err = findConference(tx, conferenceID, true)
		if err != nil {
			return err
		}
		if c.OwnerID != userID {
			return apperrors.ErrForbidden
		}
		if c.Status != conferences.Scheduled {
			return apperrors.New(apperrors.ErrConflict, "only a scheduled conference can be rescheduled")
		}
		if err = conferences.ValidateSchedule(&request.ScheduledAt, request.PlannedDurationMin, time.Now()); err != nil {
			return err
		}
		if err = tx.Model(&c).Updates(map[string]any{"scheduled_at": request.ScheduledAt.UTC(), "planned_duration_min": request.PlannedDurationMin}).Error; err != nil {
			return err
		}
		c, err = findConference(tx, conferenceID, false)
		return err
	})
	return c, err
}

// Filter before pagination; filtering an already paged roster could hide the
// next page behind a short page made up of waiting/rejected memberships.
func (r *ConferenceRepository) ParticipantsVisible(ctx context.Context, id, userID string, limit, offset int) ([]conferences.Participant, error) {
	rows := []conferences.Participant{}
	actor, err := findMembership(r.db.WithContext(ctx), id, userID)
	if err != nil {
		return rows, membershipError(err)
	}
	if !actor.CanReadHistory() {
		return rows, apperrors.ErrForbidden
	}
	q := r.db.WithContext(ctx).Where("conference_id = ?", id)
	if actor.Role != conferences.Owner && actor.Role != conferences.CoHost {
		q = q.Where("admission_state = 'admitted'")
	}
	err = q.Order("created_at ASC, id ASC").Limit(limit).Offset(offset).Find(&rows).Error
	return rows, err
}

const timelineDate = "COALESCE(conferences.finished_at, conferences.scheduled_at, conferences.created_at)"

func (r *ConferenceRepository) Timeline(ctx context.Context, userID string, query conferences.TimelineQuery) ([]conferences.Conference, error) {
	if err := query.Validate(); err != nil {
		return nil, err
	}
	q := r.db.WithContext(ctx).Model(&conferences.Conference{}).
		Joins("JOIN conference_participants p ON p.conference_id = conferences.id AND p.user_id = ?", userID).
		Where("p.admission_state IN ('admitted','waiting')")
	switch query.View {
	case "upcoming":
		q = q.Where("conferences.status IN ('created','scheduled')")
	case "active":
		q = q.Where("conferences.status = 'active'")
	case "past":
		q = q.Where("conferences.status IN ('finished','cancelled') AND p.admission_state = 'admitted' AND p.status IN ('joined','left')")
	}
	if query.Scope == "owned" {
		q = q.Where("conferences.owner_id = ?", userID)
	}
	if query.Scope == "participating" {
		q = q.Where("conferences.owner_id <> ?", userID)
	}
	if query.Status != "" {
		q = q.Where("conferences.status = ?", query.Status)
	}
	if query.From != nil {
		q = q.Where(timelineDate+" >= ?", *query.From)
	}
	if query.To != nil {
		q = q.Where(timelineDate+" <= ?", *query.To)
	}
	cursor, err := conferences.DecodeTimelineCursor(query.Cursor)
	if err != nil {
		return nil, err
	}
	if cursor != nil {
		q = q.Where("("+timelineDate+", conferences.id) < (?, ?)", cursor.At, cursor.ID)
	}
	rows := []conferences.Conference{}
	err = q.Select("conferences.*").Order(timelineDate + " DESC, conferences.id DESC").Limit(query.Limit + 1).Find(&rows).Error
	return rows, err
}

func (r *ConferenceRepository) History(ctx context.Context, conferenceID, userID string) (conferences.HistoryView, error) {
	var result conferences.HistoryView
	// A shared conference lifecycle lock prevents kick/finish/admission from
	// changing authorization while the bounded historical snapshot is assembled.
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		c, err := findConference(tx, conferenceID, true)
		if err != nil {
			return err
		}
		actor, err := findMembership(tx, conferenceID, userID)
		if err != nil {
			return membershipError(err)
		}
		if !actor.CanReadHistory() {
			return apperrors.ErrForbidden
		}
		if c.Status != conferences.Finished && c.Status != conferences.Cancelled {
			return apperrors.New(apperrors.ErrConflict, "conference history is available after closure")
		}
		owner, err := findMembership(tx, conferenceID, c.OwnerID)
		if err != nil {
			return err
		}
		result.Conference, result.Owner = c.View(), conferences.HistoryOwner{ID: c.OwnerID, DisplayName: owner.DisplayName}
		result.ChatAvailable, result.ChatReadOnly = true, true
		if c.StartedAt != nil && c.FinishedAt != nil {
			seconds := max(int64(c.FinishedAt.Sub(*c.StartedAt).Seconds()), 0)
			result.DurationSec = &seconds
		}
		q := tx.Model(&conferences.Participant{}).Where("conference_id = ?", conferenceID)
		if actor.Role != conferences.Owner && actor.Role != conferences.CoHost {
			q = q.Where("admission_state = 'admitted'")
		}
		if err = q.Count(&result.ParticipantCount).Error; err != nil {
			return err
		}
		rows := []conferences.Participant{}
		if err = q.Order("created_at ASC, id ASC").Limit(100).Find(&rows).Error; err != nil {
			return err
		}
		result.Participants = make([]conferences.ParticipantView, 0, len(rows))
		for _, p := range rows {
			result.Participants = append(result.Participants, p.View())
		}
		result.ParticipantsTruncated = result.ParticipantCount > int64(len(rows))
		return tx.Table("record").Select(`COUNT(*) AS total,
			COUNT(*) FILTER (WHERE status IN ('ready','partial_ready')) AS ready,
			COUNT(*) FILTER (WHERE status IN ('starting','recording','degraded','stopping','finalizing','uploading')) AS processing,
			COUNT(*) FILTER (WHERE status IN ('failed','cancelled')) AS failed`).
			Where("platform_conference_id = ? AND mode = 'composite'", conferenceID).Scan(&result.Recordings).Error
	})
	return result, err
}
