// Package content координирует изолированные фоновые STT/AI-задачи и доступ к их результатам.
package content

import (
	"context"
	domain "github.com/janickiy/go-recorder/internal/domain/content"
	"github.com/janickiy/go-recorder/internal/domain/jobs"
	"time"
)

// Config ограничивает расходы, размер результатов и время внешней обработки.
// TranscriptionEnabled/AIEnabled независимо отключают передачу содержимого провайдерам.
type Config struct {
	// Enable flags независимо разрешают передачу audio/text внешним адаптерам.
	TranscriptionEnabled, AIEnabled bool
	// Duration/segment limits ограничивают eligible запись и размер response.
	MaxDurationSec, MaxSegments int
	// Byte limit учитывает UTF-8 bytes текста, а не число символов.
	MaxTranscriptBytes int64
	// Chunk/worker budgets ограничивают вход операции и число параллельных AI calls.
	ChunkRunes, MaxChunks, AIConcurrency int
	// Deadline конечен для job; cooldown препятствует дорогому ручному replay.
	STTTimeout, AITimeout, ReprocessCooldown time.Duration
	// Reprocess считает новые поколения; Attempts — временные повторы поколения.
	MaxReprocess, MaxAttempts int
}

// DefaultConfig возвращает безопасные исходные лимиты; внешняя обработка выключена.
// @return настройки с конечными deadline, размером и числом повторных поколений.
func DefaultConfig() Config {
	return Config{MaxDurationSec: 7200, MaxSegments: 10000, MaxTranscriptBytes: 8 << 20, ChunkRunes: 12000, MaxChunks: 256, AIConcurrency: 2, STTTimeout: 10 * time.Minute, AITimeout: 2 * time.Minute, ReprocessCooldown: 5 * time.Minute, MaxReprocess: 3, MaxAttempts: 5}
}

// AudioSource извлекает ограниченное аудио из приватного объекта после проверки DB source.
type AudioSource interface {
	// Open выделяет временные ресурсы и возвращает обязательную функцию очистки.
	// @args ctx — deadline; source — серверные метаданные готовой записи.
	// @return audio с Reader/Cleanup или безопасную ошибку обработки.
	Open(context.Context, domain.RecordingSource) (domain.Audio, error)
}

// Repository объединяет проверки истории с атомарными переходами поколения и outbox.
// Каждый worker commit обязан проверять lease и generation, а read — live policy.
type Repository interface {
	// Transcript читает nullable metadata после проверки текущей history policy.
	// @args ctx — DB deadline; строки — userID, conferenceID, recordingID.
	// @return metadata, право owner/cohost на повторную обработку и ошибка.
	Transcript(context.Context, string, string, string) (*domain.Transcript, bool, error)
	// Segments выдаёт упорядоченный текст только ready текущего поколения.
	// @args ctx — deadline; строки — user/meeting/recording; int — limit/offset.
	// @return ограниченная страница, total и ошибка доступа/БД.
	Segments(context.Context, string, string, string, int, int) (domain.SegmentPage, error)
	// Summary читает только результат текущего transcript generation.
	// @args ctx — deadline; строки — userID, conferenceID, recordingID.
	// @return nullable summary, право reprocess и ошибка.
	Summary(context.Context, string, string, string) (*domain.Summary, bool, error)
	// QueueTranscript атомарно проверяет роль/cooldown и создаёт новое поколение/job.
	// @args ctx — deadline; строки — user/meeting/recording; maximum/cooldown/attempts — бюджеты.
	// @return queued metadata либо безопасный conflict/forbidden.
	QueueTranscript(context.Context, string, string, string, int, time.Duration, int) (*domain.Transcript, error)
	// QueueSummary сохраняет regenerate для ready transcript без HTTP AI-вызова.
	// @args ctx — deadline; строки — user/meeting/recording; maximum/cooldown/attempts — бюджеты.
	// @return queued summary или ошибка актуальных прав/лимитов.
	QueueSummary(context.Context, string, string, string, int, time.Duration, int) (*domain.Summary, error)
	// StartTranscript захватывает processing поколение с job lease fencing.
	// @args ctx — DB deadline; job — server-owned outbox и токен аренды.
	// @return metadata, private recording source и ошибка/ErrSkip.
	StartTranscript(context.Context, jobs.Job) (domain.Transcript, domain.RecordingSource, error)
	// SaveTranscript сохраняет validated segments и следующие jobs одной транзакцией.
	// @args ctx/job — deadline/lease; transcript/result — поколение/проверенный output;
	// string — имя provider; bool — включён ли AI; int — retry budget следующих jobs.
	// @return ошибка commit/fencing без изменения recording state.
	SaveTranscript(context.Context, jobs.Job, domain.Transcript, domain.TranscriptionResult, string, bool, int) error
	// StartSummary выдаёт согласованные source segments поколения для отдельного AI call.
	// @args ctx — deadline; job — leased summarize task.
	// @return processing summary, сегменты и ошибка/ErrSkip.
	StartSummary(context.Context, jobs.Job) (domain.Summary, []domain.Segment, error)
	// SaveSummary фиксирует schema-validated JSON и ready notification атомарно.
	// @args ctx/job — deadline/lease; summary/output — поколение/проверенный результат;
	// строки — provider/model/promptVersion/schemaVersion; int — delivery retry budget.
	// @return ошибка commit/fencing; recording/transcript не меняются.
	SaveSummary(context.Context, jobs.Job, domain.Summary, domain.SummaryOutput, string, string, string, string, int) error
	// FailJob сохраняет terminal content failure только для действующей аренды/поколения.
	// @args ctx — свежий DB deadline; job — failed leased task; string — safe технический код.
	// @return ошибка фиксации failure или ErrLeaseLost.
	FailJob(context.Context, jobs.Job, string) error
	// Search применяет live membership policy внутри FTS до ranking/пагинации.
	// @args ctx — deadline; string — userID; query — проверенные фильтры/bounds.
	// @return разрешённая plain-text страница и ошибка БД.
	Search(context.Context, string, domain.SearchQuery) (domain.SearchPage, error)
}
