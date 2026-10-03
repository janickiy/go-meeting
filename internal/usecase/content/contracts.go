// Пакет content координирует изолированные фоновые задачи распознавания и ИИ и доступ к результатам.
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
	// Флаги включения независимо разрешают передачу аудио и текста внешним адаптерам.
	TranscriptionEnabled, AIEnabled bool
	// Пределы длительности и сегментов ограничивают допустимую запись и размер ответа.
	MaxDurationSec, MaxSegments int
	// Предел объёма учитывает байты текста UTF-8, а не число символов.
	MaxTranscriptBytes int64
	// Бюджеты частей и работников ограничивают входные данные и число параллельных вызовов ИИ.
	ChunkRunes, MaxChunks, AIConcurrency int
	// Deadline конечен для job; cooldown препятствует дорогому ручному replay.
	STTTimeout, AITimeout, ReprocessCooldown time.Duration
	// Reprocess считает новые поколения; Attempts — временные повторы поколения.
	MaxReprocess, MaxAttempts int
}

// DefaultConfig возвращает безопасные исходные лимиты; внешняя обработка выключена.
// @return настройки с конечными сроками выполнения, размером и числом повторных поколений.
func DefaultConfig() Config {
	return Config{MaxDurationSec: 7200, MaxSegments: 10000, MaxTranscriptBytes: 8 << 20, ChunkRunes: 12000, MaxChunks: 256, AIConcurrency: 2, STTTimeout: 10 * time.Minute, AITimeout: 2 * time.Minute, ReprocessCooldown: 5 * time.Minute, MaxReprocess: 3, MaxAttempts: 5}
}

// AudioSource извлекает ограниченное аудио из приватного объекта после проверки источника в БД.
type AudioSource interface {
	// Open выделяет временные ресурсы и возвращает обязательную функцию очистки.
	// @args ctx — срок выполнения; source — серверные метаданные готовой записи.
	// @return audio с Reader/Cleanup или безопасную ошибку обработки.
	Open(context.Context, domain.RecordingSource) (domain.Audio, error)
}

// Repository объединяет проверки истории с атомарными переходами поколения и исходящей очередью.
// Каждая фиксация воркера проверяет аренду и поколение, каждое чтение — актуальную политику доступа.
type Repository interface {
	// Transcript читает необязательные метаданные после проверки текущих прав на историю.
	// @args ctx — срок выполнения операции БД; строки — userID, conferenceID, recordingID.
	// @return метаданные, право владельца или соведущего на повторную обработку и ошибка.
	Transcript(context.Context, string, string, string) (*domain.Transcript, bool, error)
	// Segments выдаёт упорядоченный текст только ready текущего поколения.
	// @args ctx — срок выполнения; строки — пользователь, встреча и запись; int — размер страницы и смещение.
	// @return ограниченная страница, total и ошибка доступа/БД.
	Segments(context.Context, string, string, string, int, int) (domain.SegmentPage, error)
	// Summary читает только результат текущего поколения расшифровки.
	// @args ctx — срок выполнения; строки — userID, conferenceID, recordingID.
	// @return nullable summary, право reprocess и ошибка.
	Summary(context.Context, string, string, string) (*domain.Summary, bool, error)
	// QueueTranscript атомарно проверяет роль/cooldown и создаёт новое поколение/job.
	// @args ctx — срок выполнения; строки — пользователь, встреча и запись; maximum/cooldown/attempts — бюджеты.
	// @return метаданные поставленного в очередь задания либо безопасную ошибку конфликта или запрета доступа.
	QueueTranscript(context.Context, string, string, string, int, time.Duration, int) (*domain.Transcript, error)
	// QueueSummary сохраняет запрос повторного построения сводки для готовой расшифровки без вызова ИИ в HTTP.
	// @args ctx — срок выполнения; строки — пользователь, встреча и запись; maximum/cooldown/attempts — бюджеты.
	// @return queued summary или ошибка актуальных прав/лимитов.
	QueueSummary(context.Context, string, string, string, int, time.Duration, int) (*domain.Summary, error)
	// StartTranscript захватывает обрабатываемое поколение с проверкой действующей аренды задания.
	// @args ctx — срок выполнения операции БД; job — серверная исходящая очередь и токен аренды.
	// @return метаданные, приватный источник записи и ошибку либо ErrSkip.
	StartTranscript(context.Context, jobs.Job) (domain.Transcript, domain.RecordingSource, error)
	// SaveTranscript сохраняет проверенные сегменты и следующие задания одной транзакцией.
	// @args ctx/job — срок выполнения и аренда; transcript/result — поколение и проверенный результат;
	// string задаёт имя провайдера; bool — включение ИИ; int — бюджет повторов следующих заданий.
	// @return ошибка фиксации транзакции или потеря актуальности аренды без изменения состояния записи.
	SaveTranscript(context.Context, jobs.Job, domain.Transcript, domain.TranscriptionResult, string, bool, int) error
	// StartSummary выдаёт согласованные исходные сегменты поколения для отдельного вызова ИИ.
	// @args ctx — срок выполнения; job — арендованное задание формирования резюме.
	// @return processing summary, сегменты и ошибка/ErrSkip.
	StartSummary(context.Context, jobs.Job) (domain.Summary, []domain.Segment, error)
	// SaveSummary атомарно фиксирует JSON, проверенный по схеме, и уведомление о готовности.
	// @args ctx/job — срок выполнения и аренда; summary/output — поколение и проверенный результат;
	// строки задают provider/model/promptVersion/schemaVersion; int — бюджет повторов доставки.
	// @return ошибка фиксации транзакции или потеря актуальности аренды; запись и расшифровка не меняются.
	SaveSummary(context.Context, jobs.Job, domain.Summary, domain.SummaryOutput, string, string, string, string, int) error
	// FailJob сохраняет окончательный сбой содержимого только для действующих аренды и поколения.
	// @args ctx — новый срок выполнения операции БД; job — арендованное задание с ошибкой; string — безопасный технический код.
	// @return ошибка фиксации failure или ErrLeaseLost.
	FailJob(context.Context, jobs.Job, string) error
	// Search проверяет актуальное членство внутри полнотекстового поиска до ранжирования и пагинации.
	// @args ctx — срок выполнения; string — userID; query — проверенные фильтры и границы.
	// @return разрешённая plain-text страница и ошибка БД.
	Search(context.Context, string, domain.SearchQuery) (domain.SearchPage, error)
}
