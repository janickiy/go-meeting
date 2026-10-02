package integrations

import (
	"context"
	"strings"
	"testing"
	"time"

	d "github.com/janickiy/go-recorder/internal/domain/integrations"
	"github.com/janickiy/go-recorder/internal/domain/jobs"
	n "github.com/janickiy/go-recorder/internal/domain/notifications"
)

// TestEmailEscapesMeetingContent проверяет HTML escaping и отсутствие исполнения пользовательского содержимого.
// @parameters: t — контекст теста.
func TestEmailEscapesMeetingContent(t *testing.T) {
	s, err := NewService(nil, d.Providers{}, nil, Options{PublicURL: "https://meet.example"})
	if err != nil {
		t.Fatal(err)
	}
	m, err := s.RenderEmail("stable", "user@example.org", "conference.invited", `<script>alert("x")</script>`, "https://meet.example/app/conferences/id")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(m.HTML, "<script>") || !strings.Contains(m.HTML, "&lt;script&gt;") || !strings.Contains(m.Text, "<script>") {
		t.Fatal("unsafe escaping")
	}
	if m.IdempotencyKey != "stable" || strings.Contains(m.HTML, "transcript") {
		t.Fatal("wrong payload")
	}
}

// linkRepository изолирует live delivery projection для проверки frontend links без DB/media.
type linkRepository struct {
	Repository
	kind string
}

// Delivery возвращает разрешённый transcript/summary-ready факт.
// @parameters: ctx — тестовый контекст; job — fake job.
// @return: минимальная безопасная delivery projection.
func (r linkRepository) Delivery(ctx context.Context, job jobs.Job) (Delivery, error) {
	p := d.DefaultPreferences("user")
	p.Email = true
	return Delivery{Allowed: true, Preferences: p, Email: "user@example.org", Notification: n.Notification{Type: r.kind, Payload: n.Payload{RecordingID: "recording-id"}}}, nil
}

// Conference предоставляет идентификатор и название без зависимости от media.
// @parameters: ctx — тестовый контекст; id — conference ID.
// @return: projection для URL.
func (r linkRepository) Conference(ctx context.Context, id string) (ConferenceSnapshot, error) {
	return ConferenceSnapshot{ID: "conference-id", Title: "Meeting"}, nil
}

// CompleteDelivery имитирует уже проверенную запись результата.
// @parameters: ctx — контекст; job — задание; status/code — безопасный исход.
// @return: nil для isolated link test.
func (r linkRepository) CompleteDelivery(ctx context.Context, job jobs.Job, status, code string) error {
	return nil
}

// capturedEmail сохраняет только последнее тестовое сообщение.
type capturedEmail struct{ message d.EmailMessage }

// Send фиксирует сообщение для проверки ссылки.
// @parameters: ctx — контекст; message — rendered email.
// @return: nil, платный провайдер не вызывается.
func (e *capturedEmail) Send(ctx context.Context, message d.EmailMessage) error {
	e.message = message
	return nil
}

// TestDeliveryUsesExistingFrontendRoutes проверяет deep-link конкретной записи вместо несуществующего /app пути.
// @parameters: t — контекст теста.
func TestDeliveryUsesExistingFrontendRoutes(t *testing.T) {
	for _, kind := range []string{"transcript.ready", "summary.ready"} {
		email := &capturedEmail{}
		s, err := NewService(linkRepository{kind: kind}, d.Providers{Email: email, Capabilities: d.Capabilities{Email: "mock", Push: "noop", Calendar: "noop"}}, nil, Options{PublicURL: "https://meet.example"})
		if err != nil {
			t.Fatal(err)
		}
		if err = s.Handle(context.Background(), jobs.Job{Kind: "integrations.delivery", ConferenceID: "conference-id", Payload: []byte(`{"channel":"email"}`)}); err != nil {
			t.Fatal(err)
		}
		tab := "transcript"
		if kind == "summary.ready" {
			tab = "summary"
		}
		if !strings.Contains(email.message.Text, "https://meet.example/conferences/conference-id?recording=recording-id&tab="+tab) || strings.Contains(email.message.Text, "/app/conferences/") {
			t.Fatal("broken notification route", email.message.Text)
		}
	}
}

// calendarLinkRepository задаёт минимальный state одной запланированной встречи.
type calendarLinkRepository struct{ Repository }

// AcquireCalendar предоставляет isolated mutex substitute без обращения к PostgreSQL.
// @parameters: ctx — контекст; id — область конференции.
// @return: тестовая release функция и nil.
func (calendarLinkRepository) AcquireCalendar(ctx context.Context, id string) (func(), error) {
	return func() {}, nil
}

// Conference возвращает расписание версии три.
// @parameters: ctx — контекст; id — conference UUID.
// @return: snapshot встречи.
func (calendarLinkRepository) Conference(ctx context.Context, id string) (ConferenceSnapshot, error) {
	at := time.Now().Add(time.Hour)
	return ConferenceSnapshot{ID: "conference", OwnerID: "owner", InviteCode: "invite-code", IntegrationVersion: 3, ScheduledAt: &at, Status: "scheduled"}, nil
}

// Connections возвращает явно mock подключение без provider tokens.
// @parameters: ctx — контекст; userID — владелец.
// @return: mock connection.
func (calendarLinkRepository) Connections(ctx context.Context, userID string) ([]d.CalendarConnection, error) {
	return []d.CalendarConnection{{ID: "connection", UserID: userID, Provider: "mock", CalendarID: "primary", Status: "connected"}}, nil
}

// MappingsForSync моделирует первоначальную внешнюю синхронизацию.
// @parameters: ctx — контекст; id — conference UUID.
// @return: пустой mapping list и nil.
func (calendarLinkRepository) MappingsForSync(ctx context.Context, id string) ([]d.CalendarMapping, error) {
	return nil, nil
}

// SaveMapping фиксирует successful provider operation без DB в route-unit test.
// @parameters: ctx — контекст; job — версия; mapping — результат.
// @return: nil.
func (calendarLinkRepository) SaveMapping(ctx context.Context, job jobs.Job, mapping d.CalendarMapping) error {
	return nil
}

// capturedCalendar изолирует создание calendar event для проверки ссылки и fencing metadata.
type capturedCalendar struct {
	d.CalendarProvider
	event d.CalendarEvent
}

// CreateEvent сохраняет запрос и назначает тестовый внешний ID.
// @parameters: ctx — контекст; credentials — mock server credentials; event — запрос.
// @return: event с ID и nil.
func (c *capturedCalendar) CreateEvent(ctx context.Context, credentials d.CalendarCredentials, event d.CalendarEvent) (d.CalendarEvent, error) {
	c.event = event
	event.ID = "external"
	return event, nil
}

// TestCalendarInviteRouteAndSourceVersion проверяет /i invite route и внешнюю version metadata.
// @parameters: t — контекст теста.
func TestCalendarInviteRouteAndSourceVersion(t *testing.T) {
	calendar := &capturedCalendar{}
	service, err := NewService(calendarLinkRepository{}, d.Providers{Calendar: calendar, Capabilities: d.Capabilities{Email: "noop", Push: "noop", Calendar: "mock"}}, nil, Options{PublicURL: "https://meet.example"})
	if err != nil {
		t.Fatal(err)
	}
	if err = service.Handle(context.Background(), jobs.Job{Kind: "integrations.calendar", ConferenceID: "conference", Version: 3, Payload: []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
	if calendar.event.JoinURL != "https://meet.example/i/invite-code" || calendar.event.SourceVersion != 3 {
		t.Fatal("broken calendar route/version")
	}
}

// TestIntegrationConfiguration проверяет bounded offsets и backwards-compatible noop запуск без публичного URL.
// @parameters: t — контекст теста.
func TestIntegrationConfiguration(t *testing.T) {
	noop := d.Providers{Capabilities: d.Capabilities{Email: "noop", Push: "noop", Calendar: "noop"}}
	if _, err := NewService(nil, noop, nil, Options{}); err != nil {
		t.Fatal(err)
	}
	for _, options := range []Options{{PublicURL: "javascript:alert(1)"}, {PublicURL: "https://meet.example", ReminderOffsets: []time.Duration{-time.Second}}, {PublicURL: "https://meet.example", ReminderOffsets: []time.Duration{time.Hour, time.Hour}}, {PublicURL: "https://meet.example", ProviderTimeout: 6 * time.Minute}} {
		if _, err := NewService(nil, d.Providers{}, nil, options); err == nil {
			t.Fatal("invalid config accepted")
		}
	}
	p := d.DefaultPreferences("user")
	if p.Email || p.Push || !p.Allows("summary.ready") || p.Allows("unknown") {
		t.Fatal("privacy defaults")
	}
}
