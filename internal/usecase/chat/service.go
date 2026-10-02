package chat

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	domain "github.com/janickiy/go-recorder/internal/domain/chat"
	"github.com/janickiy/go-recorder/internal/domain/realtime"
)

// Repository задаёт контракт зависимого компонента Repository в постоянном чате и приватных вложениях; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
// @params
//   - List: операция список с контрактом, описанным у метода.
//   - Send: операция Send с контрактом, описанным у метода.
//   - Edit: операция редактирование с контрактом, описанным у метода.
//   - Delete: операция удаление с контрактом, описанным у метода.
//   - ReadState: операция чтение состояние с контрактом, описанным у метода.
//   - MarkRead: операция Mark чтение с контрактом, описанным у метода.
//   - InitAttachment: операция Init вложение с контрактом, описанным у метода.
//   - ClaimUpload: операция Claim загрузка с контрактом, описанным у метода.
//   - CompleteUpload: операция Complete загрузка с контрактом, описанным у метода.
//   - AbortUpload: операция Abort загрузка с контрактом, описанным у метода.
//   - AttachmentForFinalize: операция вложение для Finalize с контрактом, описанным у метода.
//   - FinalizeAttachment: операция Finalize вложение с контрактом, описанным у метода.
//   - DownloadAttachment: операция Download вложение с контрактом, описанным у метода.
//   - CleanupCandidates: операция очистка Candidates с контрактом, описанным у метода.
//   - CompleteCleanup: операция Complete очистка с контрактом, описанным у метода.
type Repository interface {
	// List возвращает ограниченный список сообщений и вложений конференции с принятыми в данном слое фильтрами.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор пользователя, для которого выполняется операция.
	//   - аргумент 3 (string): идентификатор конференции, ограничивающий область операции.
	//   - аргумент 4 (string): непрозрачная граница продолжения предыдущей страницы.
	//   - аргумент 5 (int): предел количества обрабатываемых элементов.
	//
	// @return:
	//   - результат 1 (domain.Page): страница элементов и метаданные продолжения.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	List(context.Context, string, string, string, int) (domain.Page, error)
	// Send сохраняет сообщение чата с проверкой доступа, ответа и вложений; ключ запроса защищает повторную отправку от дубля.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор пользователя, для которого выполняется операция.
	//   - аргумент 3 (string): идентификатор конференции, ограничивающий область операции.
	//   - аргумент 4 (domain.SendRequest): входные параметры соответствующего прикладного запроса.
	//   - аргумент 5 (string): отпечаток нормализованного запроса для сравнения повторных отправок.
	//
	// @return:
	//   - результат 1 (domain.Message): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (bool): признак выполнения проверяемого условия или изменения состояния.
	//   - результат 3 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Send(context.Context, string, string, domain.SendRequest, string) (domain.Message, bool, error)
	// Edit изменяет текст собственного неудалённого сообщения с проверкой доступа и состояния встречи.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор пользователя, для которого выполняется операция.
	//   - аргумент 3 (string): идентификатор конференции, ограничивающий область операции.
	//   - аргумент 4 (string): идентификатор обрабатываемого ресурса.
	//   - аргумент 5 (string): обычный текст сообщения, подлежащий проверке или обработке.
	//
	// @return:
	//   - результат 1 (domain.Message): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Edit(context.Context, string, string, string, string) (domain.Message, error)
	// Delete мягко удаляет доступное сообщение, проверяя автора или полномочия модератора и сохраняя историю.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор пользователя, для которого выполняется операция.
	//   - аргумент 3 (string): идентификатор конференции, ограничивающий область операции.
	//   - аргумент 4 (string): идентификатор обрабатываемого ресурса.
	//
	// @return:
	//   - результат 1 (domain.Message): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Delete(context.Context, string, string, string) (domain.Message, error)
	// ReadState возвращает сохранённую границу прочтения и число доступных непрочитанных сообщений.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор пользователя, для которого выполняется операция.
	//   - аргумент 3 (string): идентификатор конференции, ограничивающий область операции.
	//
	// @return:
	//   - результат 1 (domain.ReadState): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	ReadState(context.Context, string, string) (domain.ReadState, error)
	// MarkRead продвигает сохранённое состояние прочтения; повторные и запоздалые запросы не должны уменьшать курсор.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор пользователя, для которого выполняется операция.
	//   - аргумент 3 (string): идентификатор конференции, ограничивающий область операции.
	//   - аргумент 4 (string): идентификатор сообщения внутри конференции.
	//
	// @return:
	//   - результат 1 (domain.ReadState): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	MarkRead(context.Context, string, string, string) (domain.ReadState, error)
	// InitAttachment создаёт или возвращает метаданные незавершённого вложения для безопасной повторной загрузки.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор пользователя, для которого выполняется операция.
	//   - аргумент 3 (string): идентификатор конференции, ограничивающий область операции.
	//   - аргумент 4 (domain.InitRequest): входные параметры соответствующего прикладного запроса.
	//
	// @return:
	//   - результат 1 (domain.Attachment): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (bool): признак выполнения проверяемого условия или изменения состояния.
	//   - результат 3 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	InitAttachment(context.Context, string, string, domain.InitRequest) (domain.Attachment, bool, error)
	// ClaimUpload атомарно захватывает попытку загрузки ограниченным по времени токеном.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор пользователя, для которого выполняется операция.
	//   - аргумент 3 (string): идентификатор конференции, ограничивающий область операции.
	//   - аргумент 4 (string): идентификатор обрабатываемого ресурса.
	//   - аргумент 5 (string): подписанный токен или токен владения, который необходимо проверить.
	//
	// @return:
	//   - результат 1 (domain.Attachment): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	ClaimUpload(context.Context, string, string, string, string) (domain.Attachment, error)
	// CompleteUpload сохраняет результат передачи объекта только для действующей попытки загрузки.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор пользователя, для которого выполняется операция.
	//   - аргумент 3 (string): идентификатор конференции, ограничивающий область операции.
	//   - аргумент 4 (string): идентификатор обрабатываемого ресурса.
	//   - аргумент 5 (string): подписанный токен или токен владения, который необходимо проверить.
	//   - аргумент 6 (string): контрольная сумма содержимого для проверки неизменности передачи.
	//
	// @return:
	//   - результат 1 (domain.Attachment): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	CompleteUpload(context.Context, string, string, string, string, string) (domain.Attachment, error)
	// AbortUpload освобождает только указанную попытку загрузки после ошибки.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор обрабатываемого ресурса.
	//   - аргумент 3 (string): подписанный токен или токен владения, который необходимо проверить.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	AbortUpload(context.Context, string, string) error
	// AttachmentForFinalize читает метаданные вложения, доступного владельцу для подтверждения загрузки.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор пользователя, для которого выполняется операция.
	//   - аргумент 3 (string): идентификатор конференции, ограничивающий область операции.
	//   - аргумент 4 (string): идентификатор обрабатываемого ресурса.
	//
	// @return:
	//   - результат 1 (domain.Attachment): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	AttachmentForFinalize(context.Context, string, string, string) (domain.Attachment, error)
	// FinalizeAttachment подтверждает готовность загруженного вложения к привязке к сообщению.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор пользователя, для которого выполняется операция.
	//   - аргумент 3 (string): идентификатор конференции, ограничивающий область операции.
	//   - аргумент 4 (string): идентификатор обрабатываемого ресурса.
	//   - аргумент 5 (string): серверный ключ объекта внутри приватного бакета.
	//
	// @return:
	//   - результат 1 (domain.Attachment): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	FinalizeAttachment(context.Context, string, string, string, string) (domain.Attachment, error)
	// DownloadAttachment проверяет доступ к сообщению и возвращает метаданные прикреплённого файла для скачивания.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор пользователя, для которого выполняется операция.
	//   - аргумент 3 (string): идентификатор конференции, ограничивающий область операции.
	//   - аргумент 4 (string): идентификатор обрабатываемого ресурса.
	//
	// @return:
	//   - результат 1 (domain.Attachment): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	DownloadAttachment(context.Context, string, string, string) (domain.Attachment, error)
	// CleanupCandidates выбирает ограниченную порцию просроченных вложений, учитывая действующие попытки загрузки.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (int): предел количества обрабатываемых элементов.
	//
	// @return:
	//   - результат 1 ([]domain.Attachment): собранные элементы результата; состав ограничивается параметрами операции.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	CleanupCandidates(context.Context, int) ([]domain.Attachment, error)
	// CompleteCleanup помечает завершённую очистку вложения после удаления ненужных объектов.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор обрабатываемого ресурса.
	//   - аргумент 3 (string): ключ ограничителя, блокировки или объекта в соответствующем хранилище.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	CompleteCleanup(context.Context, string, string) error
}

// Storage задаёт контракт зависимого компонента Storage в постоянном чате и приватных вложениях; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
// @params:
//   - CheckAttachmentPrivacy: операция проверка вложение Privacy с контрактом, описанным у метода.
//   - PutAttachment: операция Put вложение с контрактом, описанным у метода.
//   - StatAttachment: операция Stat вложение с контрактом, описанным у метода.
//   - AttachmentDownloadURL: операция вложение Download URL с контрактом, описанным у метода.
//   - CleanAttachmentObjects: операция Clean вложение Objects с контрактом, описанным у метода.
type Storage interface {
	// CheckAttachmentPrivacy проверяет отсутствие публичной политики бакета для приватных вложений.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	CheckAttachmentPrivacy(context.Context) error
	// PutAttachment сохраняет байты вложения по ключу, сформированному сервером.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): ключ ограничителя, блокировки или объекта в соответствующем хранилище.
	//   - аргумент 3 (io.Reader): источник содержимого либо читатель карточек записи согласно типу.
	//   - аргумент 4 (int64): размер содержимого в байтах.
	//   - аргумент 5 (string): значение contentType типа string, используемое согласно назначению этой операции.
	//   - аргумент 6 (string): контрольная сумма содержимого для проверки неизменности передачи.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	PutAttachment(context.Context, string, io.Reader, int64, string, string) error
	// StatAttachment читает фактический размер и метаданные объекта перед финализацией.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): ключ ограничителя, блокировки или объекта в соответствующем хранилище.
	//
	// @return:
	//   - результат 1 (int64): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (string): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 3 (string): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 4 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	StatAttachment(context.Context, string) (int64, string, string, error)
	// AttachmentDownloadURL создаёт краткоживущую подписанную ссылку с принудительным скачиванием файла.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): ключ ограничителя, блокировки или объекта в соответствующем хранилище.
	//   - аргумент 3 (string): проверяемое или формируемое имя файла без управляемого пользователем пути.
	//   - аргумент 4 (time.Duration): значение expiry типа time.Duration, используемое согласно назначению этой операции.
	//
	// @return:
	//   - результат 1 (string): адрес разрешённого чтения или целевого ресурса.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	AttachmentDownloadURL(context.Context, string, string, time.Duration) (string, error)
	// CleanAttachmentObjects удаляет объекты точного префикса вложения, сохраняя выигравший прикреплённый объект.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): точная граница принадлежащих операции объектов.
	//   - аргумент 3 (string): объект или значение, которое необходимо сохранить при очистке.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	CleanAttachmentObjects(context.Context, string, string) error
}

// Events задаёт контракт зависимого компонента Events в постоянном чате и приватных вложениях; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
// @params:
//   - Broadcast: операция Broadcast с контрактом, описанным у метода.
//   - SendToParticipant: операция Send в участник с контрактом, описанным у метода.
type Events interface {
	// Broadcast публикует доверенное событие для разрешённых получателей конференции.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (realtime.Envelope): конверт входящего или публикуемого события.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Broadcast(context.Context, realtime.Envelope) error
	// SendToParticipant публикует адресное событие физическим сессиям указанного участника.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор конференции, ограничивающий область операции.
	//   - аргумент 3 (string): идентификатор членства участника внутри конференции.
	//   - аргумент 4 (realtime.Envelope): конверт входящего или публикуемого события.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	SendToParticipant(context.Context, string, string, realtime.Envelope) error
}

// Service объединяет зависимости прикладного сценария и координирует его операции.
// @params:
//   - repo: хранилище постоянных данных прикладного сценария.
//   - storage: хранилище приватных файлов и метаданных объектов.
//   - events: получатель или издатель событий прикладного сценария.
//   - uploads: канал «uploads» для передачи данных или завершения ожидания.
type Service struct {
	repo    Repository
	storage Storage
	events  Events
	uploads chan struct{}
}

// NewService создаёт и связывает зависимости компонента Service, используемого в постоянном чате и приватных вложениях.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - repo (Repository): хранилище постоянных данных прикладного сценария.
//   - storage (Storage): хранилище приватных файлов и метаданных объектов.
//   - events (Events): получатель или издатель событий прикладного сценария.
//
// @return:
//   - результат 1 (*Service): созданный компонент с переданными зависимостями.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func NewService(ctx context.Context, repo Repository, storage Storage, events Events) (*Service, error) {
	check, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := storage.CheckAttachmentPrivacy(check); err != nil {
		return nil, err
	}
	return &Service{repo: repo, storage: storage, events: events, uploads: make(chan struct{}, 4)}, nil
}

// List возвращает ограниченный список сообщений и вложений конференции с принятыми в данном слое фильтрами.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - user (string): пользователь либо его идентификатор, определяющий область доступа.
//   - conference (string): конференция либо её идентификатор, ограничивающий область операции.
//   - cursor (string): непрозрачная граница продолжения предыдущей страницы.
//   - limit (int): максимальное число элементов страницы или порции обработки.
//
// @return:
//   - результат 1 (domain.Page): страница элементов и метаданные продолжения.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) List(ctx context.Context, user, conference, cursor string, limit int) (domain.Page, error) {
	return s.repo.List(ctx, user, conference, cursor, limit)
}

// Send сохраняет сообщение чата с проверкой доступа, ответа и вложений; ключ запроса защищает повторную отправку от дубля.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - user (string): пользователь либо его идентификатор, определяющий область доступа.
//   - conference (string): конференция либо её идентификатор, ограничивающий область операции.
//   - request (domain.SendRequest): входные параметры соответствующего прикладного запроса.
//
// @return:
//   - результат 1 (domain.Message): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (bool): признак выполнения проверяемого условия или изменения состояния.
//   - результат 3 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) Send(ctx context.Context, user, conference string, request domain.SendRequest) (domain.Message, bool, error) {
	request, fingerprint, err := domain.NormalizeSend(request)
	if err != nil {
		return domain.Message{}, false, err
	}
	message, created, err := s.repo.Send(ctx, user, conference, request, fingerprint)
	if err == nil {
		s.publish(ctx, "chat.message.created", message)
	}
	return message, created, err
}

// Edit изменяет текст собственного неудалённого сообщения с проверкой доступа и состояния встречи.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - user (string): пользователь либо его идентификатор, определяющий область доступа.
//   - conference (string): конференция либо её идентификатор, ограничивающий область операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//   - request (domain.EditRequest): входные параметры соответствующего прикладного запроса.
//
// @return:
//   - результат 1 (domain.Message): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) Edit(ctx context.Context, user, conference, id string, request domain.EditRequest) (domain.Message, error) {
	text, err := domain.NormalizeText(request.Text, true)
	if err != nil {
		return domain.Message{}, err
	}
	message, err := s.repo.Edit(ctx, user, conference, id, text)
	if err == nil {
		s.publish(ctx, "chat.message.updated", message)
	}
	return message, err
}

// Delete мягко удаляет доступное сообщение, проверяя автора или полномочия модератора и сохраняя историю.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - user (string): пользователь либо его идентификатор, определяющий область доступа.
//   - conference (string): конференция либо её идентификатор, ограничивающий область операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//
// @return:
//   - результат 1 (domain.Message): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) Delete(ctx context.Context, user, conference, id string) (domain.Message, error) {
	message, err := s.repo.Delete(ctx, user, conference, id)
	if err == nil {
		s.publish(ctx, "chat.message.deleted", message)
	}
	return message, err
}

// publish передаёт сохранённое изменение через транспорт событий или внутренних команд.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - kind (string): тип события, ошибки или медиа, определяющий ветку обработки.
//   - message (domain.Message): сообщение чата или безопасный текст ответа согласно указанному типу.
func (s *Service) publish(ctx context.Context, kind string, message domain.Message) {
	if s.events != nil {
		if err := s.events.Broadcast(ctx, realtime.Event(kind, message.ConferenceID, message)); err != nil {
			slog.Warn("chat realtime delivery deferred to client refresh", "conference_id", message.ConferenceID, "event_type", kind)
		}
	}
}

// ReadState возвращает сохранённую границу прочтения и число доступных непрочитанных сообщений.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - user (string): пользователь либо его идентификатор, определяющий область доступа.
//   - conference (string): конференция либо её идентификатор, ограничивающий область операции.
//
// @return:
//   - результат 1 (domain.ReadState): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) ReadState(ctx context.Context, user, conference string) (domain.ReadState, error) {
	return s.repo.ReadState(ctx, user, conference)
}

// MarkRead продвигает сохранённое состояние прочтения; повторные и запоздалые запросы не должны уменьшать курсор.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - user (string): пользователь либо его идентификатор, определяющий область доступа.
//   - conference (string): конференция либо её идентификатор, ограничивающий область операции.
//   - request (domain.ReadRequest): входные параметры соответствующего прикладного запроса.
//
// @return:
//   - результат 1 (domain.ReadState): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) MarkRead(ctx context.Context, user, conference string, request domain.ReadRequest) (domain.ReadState, error) {
	id, err := domain.UUID(request.MessageID)
	if err != nil {
		return domain.ReadState{}, err
	}
	state, err := s.repo.MarkRead(ctx, user, conference, id)
	if err == nil && s.events != nil {
		_ = s.events.SendToParticipant(ctx, conference, state.ParticipantID, realtime.Event("chat.read.updated", conference, state))
	}
	return state, err
}

// InitAttachment создаёт или возвращает метаданные незавершённого вложения для безопасной повторной загрузки.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - user (string): пользователь либо его идентификатор, определяющий область доступа.
//   - conference (string): конференция либо её идентификатор, ограничивающий область операции.
//   - request (domain.InitRequest): входные параметры соответствующего прикладного запроса.
//
// @return:
//   - результат 1 (domain.Attachment): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (bool): признак выполнения проверяемого условия или изменения состояния.
//   - результат 3 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) InitAttachment(ctx context.Context, user, conference string, request domain.InitRequest) (domain.Attachment, bool, error) {
	request, err := domain.NormalizeInit(request)
	if err != nil {
		return domain.Attachment{}, false, err
	}
	return s.repo.InitAttachment(ctx, user, conference, request)
}

// ValidateContent проверяет фактические байты вложения, размер, контрольную сумму и соответствие заявленному формату.
//
// @args
//   - data ([]byte): полезная нагрузка события или байты обрабатываемого содержимого.
//   - attachment (domain.Attachment): метаданные вложения, с которыми сверяется содержимое и путь объекта.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func ValidateContent(data []byte, attachment domain.Attachment) error {
	if int64(len(data)) != attachment.Size || len(data) == 0 || int64(len(data)) > domain.MaxAttachmentBytes {
		return apperrors.New(apperrors.ErrInvalidInput, "uploaded size differs from the declared attachment size")
	}
	detected := strings.Split(http.DetectContentType(data), ";")[0]
	expected := attachment.MimeType
	if expected == "text/csv" {
		expected = "text/plain"
	}
	if detected != expected {
		return apperrors.New(apperrors.ErrInvalidInput, "file content does not match its allowed type")
	}
	if expected == "text/plain" && (!utf8.Valid(data) || bytes.ContainsRune(data, 0)) {
		return apperrors.New(apperrors.ErrInvalidInput, "text attachments must be valid UTF-8")
	}
	if expected == "image/jpeg" || expected == "image/png" {
		cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || cfg.Width < 1 || cfg.Height < 1 || int64(cfg.Width)*int64(cfg.Height) > 40_000_000 {
			return apperrors.New(apperrors.ErrInvalidInput, "invalid image or image dimensions exceed the limit")
		}
	}
	return nil
}

// Upload принимает ограниченное тело загрузки, проверяет содержимое и сохраняет объект действующей попытки.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - user (string): пользователь либо его идентификатор, определяющий область доступа.
//   - conference (string): конференция либо её идентификатор, ограничивающий область операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//   - reader (io.Reader): источник содержимого либо читатель карточек записи согласно типу.
//
// @return:
//   - result (domain.Attachment): значение, подготовленное операцией для вызывающей стороны.
//   - err (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) Upload(ctx context.Context, user, conference, id string, reader io.Reader) (result domain.Attachment, err error) {
	select {
	case s.uploads <- struct{}{}:
		defer /* Вложенный обработчик выполняет выделенный шаг обработки в постоянном чате и приватных вложениях, используя состояние окружающей функции.

		 */func() { <-s.uploads }()
	default:
		return result, apperrors.New(apperrors.ErrUnavailable, "attachment upload capacity is busy")
	}
	op, cancel := context.WithTimeout(ctx, 75*time.Second)
	defer cancel()
	token := uuid.NewString()
	a, err := s.repo.ClaimUpload(op, user, conference, id, token)
	if err != nil {
		return result, err
	}
	defer /* Вложенный обработчик выполняет выделенный шаг обработки в постоянном чате и приватных вложениях, используя состояние окружающей функции.

	 */func() {
		if err != nil && a.UploadedAt == nil {
			cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_ = s.repo.AbortUpload(cleanup, id, token)
		}
	}()
	data, err := io.ReadAll(io.LimitReader(reader, domain.MaxAttachmentBytes+1))
	if err != nil {
		return result, apperrors.New(apperrors.ErrInvalidInput, "attachment upload was interrupted")
	}
	if err = ValidateContent(data, a); err != nil {
		return result, err
	}
	sum := sha256.Sum256(data)
	checksum := hex.EncodeToString(sum[:])
	if a.UploadedAt != nil {
		if a.Checksum != checksum {
			return result, apperrors.New(apperrors.ErrConflict, "attachment already contains different bytes")
		}
		return a, nil
	}
	if err = s.storage.PutAttachment(op, a.ObjectKey, bytes.NewReader(data), int64(len(data)), a.MimeType, checksum); err != nil {
		return result, apperrors.ErrUnavailable
	}
	return s.repo.CompleteUpload(op, user, conference, id, token, checksum)
}

// FinalizeAttachment подтверждает готовность загруженного вложения к привязке к сообщению.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - user (string): пользователь либо его идентификатор, определяющий область доступа.
//   - conference (string): конференция либо её идентификатор, ограничивающий область операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//
// @return:
//   - результат 1 (domain.Attachment): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) FinalizeAttachment(ctx context.Context, user, conference, id string) (domain.Attachment, error) {
	a, err := s.repo.AttachmentForFinalize(ctx, user, conference, id)
	if err != nil {
		return a, err
	}
	op, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	size, mime, checksum, err := s.storage.StatAttachment(op, a.ObjectKey)
	if err != nil {
		return domain.Attachment{}, apperrors.ErrUnavailable
	}
	if size != a.Size || mime != a.MimeType || checksum != a.Checksum {
		return domain.Attachment{}, apperrors.New(apperrors.ErrConflict, "stored attachment validation failed")
	}
	return s.repo.FinalizeAttachment(ctx, user, conference, id, a.ObjectKey)
}

// Download проверяет доступ к прикреплённому файлу и выдаёт временную подписанную ссылку чтения.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - user (string): пользователь либо его идентификатор, определяющий область доступа.
//   - conference (string): конференция либо её идентификатор, ограничивающий область операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//
// @return:
//   - результат 1 (string): адрес разрешённого чтения или целевого ресурса.
//   - результат 2 (time.Time): временная отметка результата или окончания действия разрешения.
//   - результат 3 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) Download(ctx context.Context, user, conference, id string) (string, time.Time, error) {
	a, err := s.repo.DownloadAttachment(ctx, user, conference, id)
	if err != nil {
		return "", time.Time{}, err
	}
	op, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	url, err := s.storage.AttachmentDownloadURL(op, a.ObjectKey, a.Filename, domain.DownloadTTL)
	if err != nil {
		return "", time.Time{}, apperrors.ErrUnavailable
	}
	return url, time.Now().UTC().Add(domain.DownloadTTL), nil
}

// Cleanup обрабатывает просроченные незавершённые загрузки и удаляет принадлежащие им объекты.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) Cleanup(ctx context.Context) error {
	rows, err := s.repo.CleanupCandidates(ctx, 100)
	if err != nil {
		return err
	}
	for _, a := range rows {
		keep := ""
		if a.Status == "attached" {
			keep = a.ObjectKey
		}
		if err := s.storage.CleanAttachmentObjects(ctx, a.Prefix(), keep); err != nil {
			return err
		}
		if err := s.repo.CompleteCleanup(ctx, a.ID, a.ObjectKey); err != nil {
			return err
		}
	}
	return nil
}

// Run выполняет основной цикл компонента до завершения работы или отмены контекста.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		op, cancel := context.WithTimeout(ctx, 30*time.Second)
		err := s.Cleanup(op)
		cancel()
		if err != nil && ctx.Err() == nil {
			slog.Warn("attachment orphan cleanup will retry")
		}
	}
}
