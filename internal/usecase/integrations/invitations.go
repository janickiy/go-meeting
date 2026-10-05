package integrations

import (
	"bytes"
	"context"
	"html/template"
	"net/url"
	"strings"
	texttemplate "text/template"
	"time"

	"github.com/janickiy/go-recorder/internal/domain/conferences"
	d "github.com/janickiy/go-recorder/internal/domain/integrations"
	"github.com/janickiy/go-recorder/internal/domain/jobs"
)

type InvitationDelivery struct {
	Invitation conferences.Invitation
	Conference ConferenceSnapshot
	Organizer  string
	Allowed    bool
}

type invitationDeliveryRepository interface {
	InvitationDelivery(context.Context, jobs.Job) (InvitationDelivery, error)
	CompleteInvitation(context.Context, jobs.Job, string, string) error
}

func (s *Service) deliverInvitation(ctx context.Context, job jobs.Job) error {
	repo, ok := s.repo.(invitationDeliveryRepository)
	if !ok {
		return jobs.Error{Code: "invitation_repository"}
	}
	delivery, err := repo.InvitationDelivery(ctx, job)
	if err != nil {
		return err
	}
	if !delivery.Allowed {
		if err := repo.CompleteInvitation(ctx, job, "skipped", ""); err != nil {
			return err
		}
		return jobs.ErrSkip
	}
	if s.adapters.Capabilities.Email == "noop" || s.adapters.Email == nil {
		return jobs.Error{Code: "invitation_email_unconfigured", Retryable: true}
	}
	link := strings.TrimRight(s.options.PublicURL, "/") + "/i/" + url.PathEscape(delivery.Conference.InviteCode)
	message, err := RenderInvitationEmail(job.DedupKey, delivery.Invitation.Email, delivery.Conference.Title, delivery.Organizer, delivery.Conference.ScheduledAt, link)
	if err != nil {
		return err
	}
	op, cancel := context.WithTimeout(ctx, s.options.ProviderTimeout)
	defer cancel()
	if err := s.adapters.Email.Send(op, message); err != nil {
		return err
	}
	return repo.CompleteInvitation(ctx, job, "sent", "")
}

var invitationHTML = template.Must(template.New("meeting-invitation-v1").Parse(`<h1>Приглашение на встречу</h1><p>Вас пригласили на встречу «{{.Title}}».</p><p>Организатор: {{.Organizer}}</p>{{if .Scheduled}}<p>Время встречи: {{.Scheduled}}</p>{{end}}<p><a href="{{.URL}}">Присоединиться к встрече</a></p><p>Перейдите по ссылке и укажите своё имя. Для подключения регистрация не требуется.</p>`))
var invitationText = texttemplate.Must(texttemplate.New("meeting-invitation-v1").Parse("Приглашение на встречу\nВас пригласили на встречу «{{.Title}}».\nОрганизатор: {{.Organizer}}\n{{if .Scheduled}}Время встречи: {{.Scheduled}}\n{{end}}{{.URL}}\nПерейдите по ссылке и укажите своё имя. Для подключения регистрация не требуется.\n"))

// RenderInvitationEmail uses an explicit timezone and escaped organizer/title values.
func RenderInvitationEmail(key, to, title, organizer string, scheduled *time.Time, link string) (d.EmailMessage, error) {
	data := struct{ Title, Organizer, Scheduled, URL string }{Title: title, Organizer: organizer, URL: link}
	if scheduled != nil {
		data.Scheduled = scheduled.UTC().Format("02.01.2006 15:04 MST")
	}
	var html, text bytes.Buffer
	if err := invitationHTML.Execute(&html, data); err != nil {
		return d.EmailMessage{}, err
	}
	if err := invitationText.Execute(&text, data); err != nil {
		return d.EmailMessage{}, err
	}
	return d.EmailMessage{IdempotencyKey: key, To: to, Subject: "Приглашение на встречу", HTML: html.String(), Text: text.String()}, nil
}
