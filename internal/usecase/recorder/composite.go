package recorder

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/config"
	"github.com/janickiy/go-recorder/internal/domain/media"
	"github.com/janickiy/go-recorder/internal/domain/records"
	"github.com/janickiy/go-recorder/internal/infrastructure/composite"
	"github.com/janickiy/go-recorder/internal/infrastructure/ffmpeg"
	s3storage "github.com/janickiy/go-recorder/internal/infrastructure/storage/s3"
	"github.com/janickiy/go-recorder/internal/operations"
)

// CompositeRepository задаёт контракт зависимого компонента CompositeRepository в управлении задачами записи и её артефактами; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
// @params
//   - FindByUUID: поиск записи по UUID с контрактом, описанным у метода.
//   - ListActiveComposite: операция список Active общая запись с контрактом, описанным у метода.
//   - ClaimComposite: операция Claim общая запись с контрактом, описанным у метода.
//   - RenewComposite: операция Renew общая запись с контрактом, описанным у метода.
//   - ReleaseComposite: операция освобождение общая запись с контрактом, описанным у метода.
//   - TransitionComposite: операция переход общая запись с контрактом, описанным у метода.
//   - SaveCompositeArtifacts: операция сохранение общая запись артефакты с контрактом, описанным у метода.
//   - MarkStopping: перевод записи в состояние остановки с контрактом, описанным у метода.
//   - AddEvent: операция Add событие с контрактом, описанным у метода.
type CompositeRepository interface {
	// FindByUUID читает задачу записи по её внешнему UUID.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): внешний UUID обрабатываемой записи.
	//
	// @return:
	//   - результат 1 (records.Record): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	FindByUUID(context.Context, string) (records.Record, error)
	// ListActiveComposite возвращает активные задачи общей записи для восстановления обработки.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//
	// @return:
	//   - результат 1 ([]records.Record): собранные элементы результата; состав ограничивается параметрами операции.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	ListActiveComposite(context.Context) ([]records.Record, error)
	// ClaimComposite захватывает версионную аренду задачи общей записи за конкретным воркером.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор обрабатываемого ресурса.
	//   - аргумент 3 (string): подписанный токен или токен владения, который необходимо проверить.
	//   - аргумент 4 (string): идентификатор воркера-владельца операции.
	//   - аргумент 5 (time.Duration): срок жизни сохраняемого значения или выданного разрешения.
	//
	// @return:
	//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	ClaimComposite(context.Context, string, string, string, time.Duration) (bool, error)
	// RenewComposite продлевает аренду общей записи при совпадении владельца и версии.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор обрабатываемого ресурса.
	//   - аргумент 3 (string): подписанный токен или токен владения, который необходимо проверить.
	//   - аргумент 4 (time.Duration): срок жизни сохраняемого значения или выданного разрешения.
	//
	// @return:
	//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	RenewComposite(context.Context, string, string, time.Duration) (bool, error)
	// ReleaseComposite освобождает только действующую аренду общей записи.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор обрабатываемого ресурса.
	//   - аргумент 3 (string): подписанный токен или токен владения, который необходимо проверить.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	ReleaseComposite(context.Context, string, string) error
	// TransitionComposite условно меняет состояние общей записи, проверяя владельца и версию аренды.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор обрабатываемого ресурса.
	//   - аргумент 3 (string): подписанный токен или токен владения, который необходимо проверить.
	//   - аргумент 4 (string): состояние ресурса, ответа или фильтра выборки.
	//   - аргумент 5 (error): значение cause типа error, используемое согласно назначению этой операции.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	TransitionComposite(context.Context, string, string, string, error) error
	// SaveCompositeArtifacts сохраняет итоговые артефакты общей записи с защитой от устаревшего воркера.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор обрабатываемого ресурса.
	//   - аргумент 3 (string): подписанный токен или токен владения, который необходимо проверить.
	//   - аргумент 4 (records.RecordFile): значение final типа records.RecordFile, используемое согласно назначению этой операции.
	//   - аргумент 5 (*records.RecordFile): значение preview типа *records.RecordFile, используемое согласно назначению этой операции.
	//   - аргумент 6 ([]records.RecordSegment): доступные сегменты записи для итоговой сборки.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	SaveCompositeArtifacts(context.Context, string, string, records.RecordFile, *records.RecordFile, []records.RecordSegment) error
	// MarkStopping условно переводит задачу записи в состояние «Stopping», соблюдая ограничения жизненного цикла.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): внешний UUID обрабатываемой записи.
	//   - аргумент 3 (string): причина завершения, отказа или изменения состояния.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	MarkStopping(context.Context, string, string) error
	// AddEvent добавляет постоянное диагностическое событие жизненного цикла записи.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор связанного ресурса, заданного параметром recordUUID.
	//   - аргумент 3 (string): значение eventType типа string, используемое согласно назначению этой операции.
	//   - аргумент 4 (string): семантический источник медиа либо входной источник данных.
	//   - аргумент 5 (string): значение severity типа string, используемое согласно назначению этой операции.
	//   - аргумент 6 (string): сообщение чата или безопасный текст ответа согласно указанному типу.
	//   - аргумент 7 (string): идентификатор воркера-владельца операции.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	AddEvent(context.Context, string, string, string, string, string, string) error
}

// CompositeRegistry задаёт контракт зависимого компонента CompositeRegistry в управлении задачами записи и её артефактами; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
//   - GetOwner: операция получение владелец с контрактом, описанным у метода.
type CompositeRegistry interface {
	// GetOwner читает актуального владельца медиа-комнаты и его версию владения.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор конференции, ограничивающий область операции.
	//
	// @return:
	//   - результат 1 (media.Route): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	GetOwner(context.Context, string) (media.Route, error)
}

// CompositeOptions собирает зависимости и настройки общей записи конференции.
// @params:
//   - Repository: хранилище постоянных данных прикладного сценария.
//   - Registry: распределённый реестр воркеров и владения комнатами.
//   - S3: клиент приватного объектного хранилища MinIO/S3.
//   - StoragePath: корневой каталог локального хранения артефактов записи.
//   - FFmpegPath: значение FFmpegPath типа string, используемое согласно назначению этой операции.
//   - WorkerID: идентификатор воркера-владельца операции.
//   - InternalSecret: значение InternalSecret типа string, используемое согласно назначению этой операции.
//   - Config: настройки запуска и ограничений компонента.
//   - ConferenceLock: значение ConferenceLock типа conferenceReleaser, используемое согласно назначению этой операции.
//   - Publish: операция Publish с контрактом, описанным у метода.
//   - Logger: значение Logger типа *log.Logger, используемое согласно назначению этой операции.
type CompositeOptions struct {
	Repository     CompositeRepository
	Registry       CompositeRegistry
	S3             *s3storage.Client
	StoragePath    string
	FFmpegPath     string
	WorkerID       string
	InternalSecret string
	Config         config.CompositeConfig
	ConferenceLock conferenceReleaser
	// Publish вызывается после фиксации перехода состояния. Он должен ограничивать время
	// ввода-вывода и не вызывать этот сервис повторно.
	Publish func(context.Context, records.Record, string) error
	Logger  *log.Logger
}

// CompositeService управляет записью независимо от времени AMQP-доставки; аренда БД защищает приём, FFmpeg и публикацию артефактов.
//   - o: зависимости и настройки создаваемого компонента.
//   - composer: значение composer типа *composite.Composer, используемое согласно назначению этой операции.
//   - processor: значение processor типа *ffmpeg.PostProcessor, используемое согласно назначению этой операции.
//   - client: клиент внешнего сервиса или транспорта компонента.
//   - mu: блокировка согласованного доступа к разделяемому состоянию.
//   - ctx: контекст отмены, дедлайна и времени жизни операции.
//   - running: индекс значений running для поиска и согласования состояния.
//   - wg: счётчик принадлежащих компоненту фоновых горутин для ожидания завершения.
//   - started: логический признак started, управляющий соответствующей веткой обработки.
type CompositeService struct {
	o         CompositeOptions
	composer  *composite.Composer
	processor *ffmpeg.PostProcessor
	client    *http.Client
	mu        sync.Mutex
	ctx       context.Context
	running   map[string]struct{}
	wg        sync.WaitGroup
	started   bool
	draining  bool
}

// NewCompositeService создаёт и связывает зависимости компонента CompositeService, используемого в управлении задачами записи и её артефактами.
//
// @args
//   - o (CompositeOptions): зависимости и настройки создаваемого компонента.
//
// @return:
//   - результат 1 (*CompositeService): созданный компонент с переданными зависимостями.
func NewCompositeService(o CompositeOptions) *CompositeService {
	if o.Logger == nil {
		o.Logger = log.Default()
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.ResponseHeaderTimeout = 5 * time.Second
	return &CompositeService{o: o, composer: composite.NewComposer(o.FFmpegPath, o.Config.Width, o.Config.Height, o.Config.FPS, o.Config.Concurrency), processor: ffmpeg.NewPostProcessor(o.FFmpegPath), client: &http.Client{Transport: transport, CheckRedirect: /* Вложенный обработчик выполняет выделенный шаг обработки в управлении задачами записи и её артефактами, используя состояние окружающей функции.

	@args
	  - аргумент 1 (*http.Request): входящий HTTP-запрос.
	  - аргумент 2 ([]*http.Request): входящий HTTP-запрос.

	@return:
	  - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, ctx: context.Background(), running: map[string]struct{}{}}
}

// Start запускает обработку задач записи и связанных артефактов и подготавливает связанные ресурсы.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
func (s *CompositeService) Start(ctx context.Context) {
	s.mu.Lock()
	if s.started {
		s.mu.Unlock()
		return
	}
	s.ctx = ctx
	s.started = true
	s.wg.Add(1)
	s.mu.Unlock()
	go /* Вложенный обработчик выполняет выделенный шаг обработки в управлении задачами записи и её артефактами, используя состояние окружающей функции.

	 */func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.o.Config.PollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.reconcile(ctx)
			}
		}
	}()
}

// Wait ожидает завершения принадлежащих компоненту фоновых обработчиков и освобождает транспортные ресурсы.
func (s *CompositeService) Wait() { s.wg.Wait(); s.client.CloseIdleConnections() }

// Active возвращает число принадлежащих процессу задач, включая ожидание FFmpeg.
// Аргументов нет; блокировка защищает только чтение локального индекса.
func (s *CompositeService) Active() int { s.mu.Lock(); defer s.mu.Unlock(); return len(s.running) }

// BeginDrain прекращает новые захваты, пока действующие записи завершаются и загружаются.
func (s *CompositeService) BeginDrain() { s.mu.Lock(); s.draining = true; s.mu.Unlock() }

// WaitContext ограничивает ожидание остановленных задач дедлайном ctx.
// Возвращает nil после освобождения транспорта или ctx.Err при истечении срока;
// отмена работы должна быть запрошена владельцем сервиса до вызова.
func (s *CompositeService) WaitContext(ctx context.Context) error {
	done := make(chan struct{})
	go func() { s.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// HandleCommand исполняет внутреннюю команду запуска или завершения записи с защитой от повторной доставки.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - command (records.Command): внутренняя команда с типом операции и серверной идентичностью ресурса.
//   - record (records.Record): задача записи с её сохранённым состоянием.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *CompositeService) HandleCommand(ctx context.Context, command records.Command, record records.Record) error {
	switch command.Type {
	case "record.start":
		if records.IsTerminalStatus(record.Status) {
			return nil
		}
		return s.launch(ctx, record)
	case "record.stop":
		if records.IsTerminalStatus(record.Status) {
			return nil
		}
		if record.Status == records.StatusStarting || record.Status == records.StatusRecording {
			if err := s.o.Repository.MarkStopping(ctx, record.UUID, command.Reason); err != nil && !errors.Is(err, records.ErrRecordStateChanged) {
				return err
			}
			record.Status = records.StatusStopping
			s.publish(ctx, record, "recording.stopping")
		}
		// Команда может попасть в другую реплику. Владелец замечает stopping при опросе БД;
		// запуск разрешён только при отсутствии действующего владельца.
		return s.launch(ctx, record)
	default:
		return fmt.Errorf("unknown composite command")
	}
}

// reconcile восстанавливает согласованность сохранённого состояния и действующих ресурсов после сбоев.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
func (s *CompositeService) reconcile(ctx context.Context) {
	queryCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	records, err := s.o.Repository.ListActiveComposite(queryCtx)
	if err != nil {
		return
	}
	for _, record := range records {
		if err := s.launch(queryCtx, record); err != nil {
			s.o.Logger.Printf("composite reconciliation record=%s failed", record.UUID)
		}
	}
}

// launch запускает обработку задачи общей записи после получения аренды.
// Синхронизирует доступ к разделяемому состоянию блокировкой.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - record (records.Record): задача записи с её сохранённым состоянием.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *CompositeService) launch(ctx context.Context, record records.Record) error {
	s.mu.Lock()
	if _, ok := s.running[record.UUID]; ok || s.draining || len(s.running) >= s.o.Config.MaxActive || s.ctx.Err() != nil {
		s.mu.Unlock()
		return nil
	}
	s.running[record.UUID] = struct{}{}
	workParent := s.ctx
	s.wg.Add(1)
	s.mu.Unlock()
	token := uuid.NewString()
	claimed, err := s.o.Repository.ClaimComposite(ctx, record.UUID, token, s.o.WorkerID, s.o.Config.LeaseTTL)
	if err != nil || !claimed {
		s.mu.Lock()
		delete(s.running, record.UUID)
		s.mu.Unlock()
		s.wg.Done()
		return err
	}
	go /* Вложенный обработчик выполняет выделенный шаг обработки в управлении задачами записи и её артефактами, используя состояние окружающей функции.
	Синхронизирует доступ к разделяемому состоянию блокировкой.

	*/func() {
		defer s.wg.Done()
		defer /* Вложенный обработчик выполняет выделенный шаг обработки в управлении задачами записи и её артефактами, используя состояние окружающей функции.
		Синхронизирует доступ к разделяемому состоянию блокировкой.

		*/func() { s.mu.Lock(); delete(s.running, record.UUID); s.mu.Unlock() }()
		workCtx, cancel := context.WithCancel(workParent)
		defer cancel()
		renewed := make(chan struct{})
		go /* Вложенный обработчик выполняет выделенный шаг обработки в управлении задачами записи и её артефактами, используя состояние окружающей функции.

		 */func() {
			defer close(renewed)
			ticker := time.NewTicker(s.o.Config.LeaseTTL / 4)
			defer ticker.Stop()
			for {
				select {
				case <-workCtx.Done():
					return
				case <-ticker.C:
					checkCtx, done := context.WithTimeout(workCtx, s.o.Config.LeaseTTL/8)
					ok, err := s.o.Repository.RenewComposite(checkCtx, record.UUID, token, s.o.Config.LeaseTTL)
					done()
					if err != nil || !ok {
						cancel()
						return
					}
				}
			}
		}()
		err := s.run(workCtx, record, token)
		if err != nil {
			s.fail(record, token, err)
		}
		cancel()
		<-renewed
		releaseCtx, done := context.WithTimeout(context.Background(), 3*time.Second)
		defer done()
		_ = s.o.Repository.ReleaseComposite(releaseCtx, record.UUID, token)
	}()
	return nil
}

// run выполняет основной цикл компонента до завершения работы или отмены контекста.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - record (records.Record): задача записи с её сохранённым состоянием.
//   - token (string): подписанный токен или токен владения, который необходимо проверить.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *CompositeService) run(ctx context.Context, record records.Record, token string) error {
	started := time.Now()
	defer func() { operations.Observe("recording", time.Since(started).Seconds()) }()
	dir := filepath.Join(s.o.StoragePath, "records", record.UUID)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	if record.Status == records.StatusRecording || (record.Status == records.StatusStarting && record.RecorderToken != nil) {
		return fmt.Errorf("recorder interrupted; completed chunks retained for recovery")
	}
	if record.Status == records.StatusStarting {
		if err := s.capture(ctx, record, token, dir); err != nil {
			return err
		}
	}
	current, err := s.o.Repository.FindByUUID(ctx, record.UUID)
	if err != nil {
		return err
	}
	if current.Status != records.StatusStopping && current.Status != records.StatusFinalizing && current.Status != records.StatusUploading {
		return fmt.Errorf("recording input ended without a stop transition")
	}
	if err := s.o.Repository.TransitionComposite(ctx, record.UUID, token, records.StatusFinalizing, nil); err != nil {
		return err
	}
	current.Status = records.StatusFinalizing
	s.publish(ctx, current, "recording.processing")
	if err := s.composer.Recover(ctx, dir); err != nil {
		return err
	}
	finalizationStart := time.Now()
	var result ffmpeg.Result
	audioOnly := record.Mode == records.ModeAudioOnly || record.Mode == records.ModeIndividualTracks
	if audioOnly {
		result, err = s.processor.FinalizeAudio(ctx, dir)
	} else {
		result, err = s.processor.FinalizeComposite(ctx, dir)
	}
	operations.Observe("finalization", time.Since(finalizationStart).Seconds())
	if err != nil {
		return err
	}
	if err := s.o.Repository.TransitionComposite(ctx, record.UUID, token, records.StatusUploading, nil); err != nil {
		return err
	}
	if s.o.S3 == nil {
		return fmt.Errorf("recording storage is unavailable")
	}
	// Неизменные ключи экземпляров защищают и хранилище: запоздалая загрузка прежнего владельца
	// не может перезаписать уже опубликованный артефакт владельца действующей аренды.
	base := filepath.ToSlash(filepath.Join("recordings", record.ConferenceID, record.UUID, "artifacts", token))
	committed := false
	defer func() {
		if !committed {
			cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.o.S3.RemovePrefix(cleanup, base+"/")
		}
	}()
	mime, fileType := "video/mp4", records.FileTypeFinalMP4
	if audioOnly {
		mime, fileType = "audio/mp4", records.FileTypeFinalAudio
	}
	finalUpload, err := s.o.S3.UploadFile(ctx, base+"/final.mp4", result.FinalPath, mime)
	if err != nil {
		return fmt.Errorf("upload final recording: %w", err)
	}
	final := records.RecordFile{FileType: fileType, Bucket: finalUpload.Bucket, ObjectKey: finalUpload.ObjectKey, FileName: "final.mp4", MimeType: mime, SizeBytes: &result.FinalSizeBytes, DurationSec: &result.DurationSec, ChecksumSHA256: &result.FinalChecksum, IsPrimary: true}
	if origin, e := composite.TimelineOrigin(dir); e == nil && origin > 0 {
		final.MetadataJSON, _ = json.Marshal(map[string]any{"timelineOriginNs": origin, "mode": record.Mode})
	}
	var preview *records.RecordFile
	if !audioOnly {
		previewUpload, err := s.o.S3.UploadFile(ctx, base+"/preview.jpg", result.PreviewPath, "image/jpeg")
		if err != nil {
			return fmt.Errorf("upload recording preview: %w", err)
		}
		preview = &records.RecordFile{FileType: records.FileTypePreviewJPG, Bucket: previewUpload.Bucket, ObjectKey: previewUpload.ObjectKey, FileName: "preview.jpg", MimeType: "image/jpeg", SizeBytes: &result.PreviewSizeBytes, ChecksumSHA256: &result.PreviewChecksum}
	}
	if record.Mode == records.ModeIndividualTracks {
		archive, size, sum, err := composite.ArchiveTracks(ctx, dir, s.o.Config.MaxBytes)
		if err != nil {
			return err
		}
		defer os.Remove(archive)
		upload, err := s.o.S3.UploadFile(ctx, base+"/tracks.zip", archive, "application/zip")
		if err != nil {
			return err
		}
		final.Related = []records.RecordFile{{FileType: records.FileTypeTracksArchive, Bucket: upload.Bucket, ObjectKey: upload.ObjectKey, FileName: "tracks.zip", MimeType: "application/zip", SizeBytes: &size, ChecksumSHA256: &sum}}
	}
	if err := s.o.Repository.SaveCompositeArtifacts(ctx, record.UUID, token, final, preview, segmentMetadata(result.Segments)); err != nil {
		return err
	}
	committed = true
	current, err = s.o.Repository.FindByUUID(ctx, record.UUID)
	operations.Event("recording_ready")
	operations.Event("recording_mode_" + record.Mode)
	if err == nil {
		s.publish(ctx, current, "recording.ready")
	}
	if s.o.ConferenceLock != nil {
		_ = s.o.ConferenceLock.Release(ctx, record.ConferenceID, record.UUID)
	}
	_ = s.o.Repository.AddEvent(ctx, record.UUID, "recording.ready", "recorder", "info", "Validated composite MP4 and preview uploaded", s.o.WorkerID)
	// Сегменты неудачных и прерванных записей сохраняются. После защищённой арендой фиксации
	// ready источником истины становится приватный MinIO, и временные файлы успешной записи
	// можно удалить; KEEP_LOCAL предназначен для диагностики и приёмочных тестов.
	if !s.o.Config.KeepLocal {
		if err := os.RemoveAll(dir); err != nil {
			s.o.Logger.Printf("composite local cleanup failed record=%s", record.UUID)
		}
	}
	return nil
}

// capture принимает поток закодированных медиа для устойчивого сохранения и последующей композиции.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - record (records.Record): задача записи с её сохранённым состоянием.
//   - token (string): подписанный токен или токен владения, который необходимо проверить.
//   - dir (string): значение dir типа string, используемое согласно назначению этой операции.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *CompositeService) capture(ctx context.Context, record records.Record, token, dir string) error {
	captureCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	route, err := s.o.Registry.GetOwner(captureCtx, record.ConferenceID)
	if err != nil {
		return fmt.Errorf("conference media worker unavailable")
	}
	endpoint, err := url.Parse(route.Endpoint)
	if err != nil || (endpoint.Scheme != "http" && endpoint.Scheme != "https") || endpoint.Host == "" || endpoint.User != nil {
		return fmt.Errorf("invalid media worker endpoint")
	}
	seconds := record.SegmentDurationSec
	if seconds < 1 || seconds > 30 {
		seconds = 5
	}
	request := media.EgressRequest{RequestID: uuid.NewString(), ConferenceID: record.ConferenceID, RecordingID: record.UUID, Route: route, SegmentDurationSec: seconds}
	data, _ := json.Marshal(request)
	httpRequest, err := http.NewRequestWithContext(captureCtx, http.MethodPost, strings.TrimRight(route.Endpoint, "/")+"/internal/media/egress", bytes.NewReader(data))
	if err != nil {
		return err
	}
	httpRequest.Header.Set("Authorization", "Bearer "+s.o.InternalSecret)
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("X-Request-ID", request.RequestID)
	response, err := s.client.Do(httpRequest)
	if err != nil {
		return fmt.Errorf("connect recording egress failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("recording egress rejected (%d)", response.StatusCode)
	}
	monitored := make(chan struct{})
	input := &activityReader{reader: response.Body}
	input.lastRead.Store(time.Now().UnixNano())
	go /* Вложенный обработчик выполняет выделенный шаг обработки в управлении задачами записи и её артефактами, используя состояние окружающей функции.

	 */func() {
		defer close(monitored)
		ticker := time.NewTicker(s.o.Config.PollInterval)
		defer ticker.Stop()
		deadline := time.NewTimer(s.o.Config.MaxDuration)
		defer deadline.Stop()
		for {
			select {
			case <-captureCtx.Done():
				return
			case <-deadline.C:
				queryCtx, done := context.WithTimeout(captureCtx, 3*time.Second)
				_ = s.o.Repository.MarkStopping(queryCtx, record.UUID, "duration_limit")
				done()
				cancel()
				return
			case <-ticker.C:
				if time.Since(time.Unix(0, input.lastRead.Load())) > 15*time.Second {
					cancel()
					return
				}
				queryCtx, done := context.WithTimeout(captureCtx, 3*time.Second)
				current, err := s.o.Repository.FindByUUID(queryCtx, record.UUID)
				done()
				if err != nil {
					cancel()
					return
				}
				if current.Status == records.StatusStopping || records.IsTerminalStatus(current.Status) {
					cancel()
					return
				}
			}
		}
	}()
	err = s.composer.Capture(captureCtx, ctx, input, dir, composite.CaptureOptions{Mode: record.Mode, SegmentDuration: time.Duration(seconds) * time.Second, MaxBytes: s.o.Config.MaxBytes, OnStarted: /* Вложенный обработчик выполняет выделенный шаг обработки в управлении задачами записи и её артефактами, используя состояние окружающей функции.


	@return:
	  - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func() error {
		if err := s.o.Repository.TransitionComposite(ctx, record.UUID, token, records.StatusRecording, nil); err != nil {
			if errors.Is(err, records.ErrRecordStateChanged) {
				current, readErr := s.o.Repository.FindByUUID(ctx, record.UUID)
				if readErr == nil && current.Status == records.StatusStopping {
					cancel()
					return nil
				}
			}
			return err
		}
		current, err := s.o.Repository.FindByUUID(ctx, record.UUID)
		if err != nil {
			return err
		}
		s.publish(ctx, current, "recording.started")
		return nil
	}})
	cancel()
	<-monitored
	if composite.OnlyEgressEnded(err) {
		current, readErr := s.o.Repository.FindByUUID(ctx, record.UUID)
		if readErr == nil && current.Status == records.StatusStopping {
			return nil
		}
	}
	return err
}

// activityReader задаёт согласованное представление данных «activity читатель» для управлении задачами записи и её артефактами.
//   - reader: источник содержимого либо читатель карточек записи согласно типу.
//   - lastRead: значение lastRead типа atomic.Int64, используемое согласно назначению этой операции.
type activityReader struct {
	reader   io.Reader
	lastRead atomic.Int64
}

// Read читает состояние задач записи и связанных артефактов для дальнейшей обработки или ответа.
//
// @args
//   - p ([]byte): байты, переданные по контракту io.Writer.
//
// @return:
//   - результат 1 (int): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *activityReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 {
		r.lastRead.Store(time.Now().UnixNano())
	}
	return n, err
}

// Close закрывает принадлежащие компоненту ресурсы и завершает связанный жизненный цикл.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *activityReader) Close() error {
	if closer, ok := r.reader.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// fail фиксирует ошибочное завершение и запускает предусмотренную очистку ресурса.
//
// @args
//   - record (records.Record): задача записи с её сохранённым состоянием.
//   - token (string): подписанный токен или токен владения, который необходимо проверить.
//   - cause (error): значение cause типа error, используемое согласно назначению этой операции.
func (s *CompositeService) fail(record records.Record, token string, cause error) {
	operations.Event("recording_failed")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Полная диагностика FFmpeg остаётся локально; клиент получает ограниченное безопасное сообщение.
	s.o.Logger.Printf("composite record=%s failed: %v", record.UUID, cause)
	//lint:ignore ST1005 Сообщение клиенту сохраняет существующий текст ошибки.
	safe := errors.New("Conference recording failed; completed segments were retained")
	if err := s.o.Repository.TransitionComposite(ctx, record.UUID, token, records.StatusFailed, safe); err != nil {
		return
	}
	current, err := s.o.Repository.FindByUUID(ctx, record.UUID)
	if err == nil {
		s.publish(ctx, current, "recording.failed")
	}
	if s.o.ConferenceLock != nil {
		_ = s.o.ConferenceLock.Release(ctx, record.ConferenceID, record.UUID)
	}
	_ = s.o.Repository.AddEvent(ctx, record.UUID, "recording.failed", "recorder", "error", safe.Error(), s.o.WorkerID)
}

// publish передаёт сохранённое изменение через транспорт событий или внутренних команд.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - record (records.Record): задача записи с её сохранённым состоянием.
//   - event (string): конверт входящего или публикуемого события.
func (s *CompositeService) publish(ctx context.Context, record records.Record, event string) {
	if s.o.Publish == nil {
		return
	}
	publishCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := s.o.Publish(publishCtx, record, event); err != nil {
		s.o.Logger.Printf("recording event publish failed record=%s event=%s", record.UUID, event)
	}
}
