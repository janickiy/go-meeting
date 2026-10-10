package app

import (
	"context"
	"encoding/json"
	"time"

	content "github.com/janickiy/meet-space/internal/domain/content"
	integrations "github.com/janickiy/meet-space/internal/domain/integrations"
	"github.com/janickiy/meet-space/internal/operations"
)

// measuredEmail дополняет заменяемый адаптер счётчиком попыток, не копируя текст письма.
type measuredEmail struct{ integrations.EmailProvider }

// Send сохраняет контракт адаптера и измеряет одну фактическую отправку.
// @args ctx — срок выполнения; message — минимальные данные письма и стабильный ключ.
// @return исходная классифицированная ошибка без изменения политики повторов.
func (p measuredEmail) Send(ctx context.Context, message integrations.EmailMessage) (err error) {
	started := time.Now()
	defer func() { operations.ProviderCall("email", time.Since(started), err) }()
	return p.EmailProvider.Send(ctx, message)
}

// measuredPush измеряет вызовы платформенного канала без токена в метриках.
type measuredPush struct{ integrations.PushProvider }

// Send учитывает отдельную попытку доставки устройству.
// @args ctx — срок выполнения; message — разрешённый push и серверный токен.
// @return неизменённая ошибка адаптера.
func (p measuredPush) Send(ctx context.Context, message integrations.PushMessage) (err error) {
	started := time.Now()
	defer func() { operations.ProviderCall("push", time.Since(started), err) }()
	return p.PushProvider.Send(ctx, message)
}

// measuredCalendar сохраняет независимый от поставщика контракт и единый набор технических меток.
type measuredCalendar struct{ integrations.CalendarProvider }

// CreateEvent измеряет идемпотентное создание внешнего события.
// @args ctx — срок выполнения; credentials — приватные OAuth данные; event — разрешённое расписание.
// @return событие и неизменённая ошибка провайдера.
func (p measuredCalendar) CreateEvent(ctx context.Context, credentials integrations.CalendarCredentials, event integrations.CalendarEvent) (out integrations.CalendarEvent, err error) {
	started := time.Now()
	defer func() { operations.ProviderCall("calendar", time.Since(started), err) }()
	return p.CalendarProvider.CreateEvent(ctx, credentials, event)
}

// UpdateEvent измеряет версионированное обновление без публикации ID события.
// @args ctx — срок выполнения; credentials — секреты подключения; event — новое расписание.
// @return событие и исходная ошибка.
func (p measuredCalendar) UpdateEvent(ctx context.Context, credentials integrations.CalendarCredentials, event integrations.CalendarEvent) (out integrations.CalendarEvent, err error) {
	started := time.Now()
	defer func() { operations.ProviderCall("calendar", time.Since(started), err) }()
	return p.CalendarProvider.UpdateEvent(ctx, credentials, event)
}

// CancelEvent измеряет отмену соответствующего внешнего события.
// @args ctx — срок выполнения; credentials — приватные токены; event — сохранённое соответствие.
// @return исходная ошибка отмены.
func (p measuredCalendar) CancelEvent(ctx context.Context, credentials integrations.CalendarCredentials, event integrations.CalendarEvent) (err error) {
	started := time.Now()
	defer func() { operations.ProviderCall("calendar", time.Since(started), err) }()
	return p.CalendarProvider.CancelEvent(ctx, credentials, event)
}

// GetEvent измеряет чтение внешнего события, если оно потребуется сценарию сверки.
// @args ctx — срок выполнения; credentials — серверные секреты; event — идентификаторы соответствия.
// @return событие и исходная ошибка чтения.
func (p measuredCalendar) GetEvent(ctx context.Context, credentials integrations.CalendarCredentials, event integrations.CalendarEvent) (out integrations.CalendarEvent, err error) {
	started := time.Now()
	defer func() { operations.ProviderCall("calendar", time.Since(started), err) }()
	return p.CalendarProvider.GetEvent(ctx, credentials, event)
}

// measuredSTT измеряет аудио-запрос, сохраняя потоковую передачу и Name адаптера.
type measuredSTT struct{ content.TranscriptionProvider }

// Transcribe учитывает попытку распознавания без аудио или текста в метриках.
// @args ctx — срок выполнения; request — ограниченный поток WAV и ключ дедупликации.
// @return исходные сегменты либо классифицированная ошибка.
func (p measuredSTT) Transcribe(ctx context.Context, request content.TranscriptionRequest) (out content.TranscriptionResult, err error) {
	started := time.Now()
	defer func() { operations.ProviderCall("stt", time.Since(started), err) }()
	return p.TranscriptionProvider.Transcribe(ctx, request)
}

// measuredAI измеряет каждый chunk/merge, не меняя инструкции и проверку схемы.
type measuredAI struct{ content.AIProvider }

// Summarize учитывает отдельный внешний вызов без инструкции, текста и модели в метках метрик.
// @args ctx — срок выполнения; request — версия инструкции и недоверенные данные.
// @return исходный JSON, который по-прежнему обязан проверить usecase.
func (p measuredAI) Summarize(ctx context.Context, request content.AIRequest) (out json.RawMessage, err error) {
	started := time.Now()
	defer func() { operations.ProviderCall("ai", time.Since(started), err) }()
	return p.AIProvider.Summarize(ctx, request)
}
