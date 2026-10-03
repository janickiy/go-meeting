package recordsapp

import (
	"context"

	"github.com/janickiy/go-recorder/internal/domain/records"
	workerinfra "github.com/janickiy/go-recorder/internal/infrastructure/worker"
)

// Service задаёт контракт зависимого компонента Service в управлении задачами записи и её артефактами; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
// @params
//   - Start: операция запуск с контрактом, описанным у метода.
//   - Stop: операция остановка с контрактом, описанным у метода.
//   - List: операция список с контрактом, описанным у метода.
//   - CountByConference: операция количество By конференция с контрактом, описанным у метода.
//   - Read: операция чтение с контрактом, описанным у метода.
type Service interface {
	// Start запускает обработку задач записи и связанных артефактов и подготавливает связанные ресурсы.
	//
	// @args
	//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - request (records.StartRequest): входные параметры соответствующего прикладного запроса.
	//
	// @return
	//   - результат 1 (records.StartResponse): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Start(ctx context.Context, request records.StartRequest) (records.StartResponse, error)
	// Stop останавливает активную обработку задач записи и связанных артефактов и освобождает связанные ресурсы.
	//
	// @args
	//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - request (records.EndRequest): входные параметры соответствующего прикладного запроса.
	//
	// @return
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Stop(ctx context.Context, request records.EndRequest) error
	// List возвращает ограниченный список задач записи и связанных артефактов с принятыми в данном слое фильтрами.
	//
	// @args
	//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - limit (int): максимальное число элементов страницы или порции обработки.
	//   - offset (int): число элементов, пропускаемых перед началом страницы.
	//
	// @return
	//   - результат 1 ([]records.RecordCard): собранные элементы результата; состав ограничивается параметрами операции.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	List(ctx context.Context, limit int, offset int) ([]records.RecordCard, error)
	// CountByConference пакетно собирает количество и краткие карточки записей переданных конференций.
	//
	// @args
	//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - conferenceIDs ([]string): идентификаторы конференций для пакетной выборки.
	//   - status (string): состояние ресурса, ответа или фильтра выборки.
	//
	// @return
	//   - результат 1 ([]records.ConferenceRecordSummary): собранные элементы результата; состав ограничивается параметрами операции.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	CountByConference(ctx context.Context, conferenceIDs []string, status string) ([]records.ConferenceRecordSummary, error)
	// Read читает состояние задач записи и связанных артефактов для дальнейшей обработки или ответа.
	//
	// @args
	//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - uuid (string): внешний UUID обрабатываемой записи.
	//
	// @return
	//   - результат 1 (records.RecordCard): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Read(ctx context.Context, uuid string) (records.RecordCard, error)
}

// WorkerSignaler задаёт контракт зависимого компонента WorkerSignaler в управлении задачами записи и её артефактами; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
// @params:
//   - Offer: операция SDP-предложение с контрактом, описанным у метода.
type WorkerSignaler interface {
	// Offer обрабатывает или передаёт SDP-предложение действующего WebRTC-подключения.
	//
	// @args
	//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - recordID (string): внешний UUID задачи записи.
	//   - request (records.WebRTCOfferRequest): входные параметры соответствующего прикладного запроса.
	//
	// @return
	//   - результат 1 (records.WebRTCAnswerResponse): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Offer(ctx context.Context, recordID string, request records.WebRTCOfferRequest) (records.WebRTCAnswerResponse, error)
}

// Handler связывает транспортный запрос с прикладным сценарием, проверкой входных данных и формированием ответа.
// @params:
//   - service: значение service типа Service, используемое согласно назначению этой операции.
//   - workerSignaler: значение workerSignaler типа WorkerSignaler, используемое согласно назначению этой операции.
type Handler struct {
	service        Service
	workerSignaler WorkerSignaler
}

// NewHandler создаёт HTTP-обработчик.
// @args
// - service: прикладной сервис управления записью.
// - workerInternalURL: внутренний URL recorder-worker.
// @return Handler.
func NewHandler(service Service, workerInternalURL string) *Handler {
	return NewHandlerWithSignaler(service, workerinfra.NewClient(workerInternalURL))
}

// NewHandlerWithSignaler создаёт HTTP-обработчик с явно переданным клиентом сигнализации.
// @args
// - service: прикладной сервис управления записью.
// - workerSignaler: клиент сигнализации SDP сервиса recorder-worker.
// @return Handler.
func NewHandlerWithSignaler(service Service, workerSignaler WorkerSignaler) *Handler {
	return &Handler{
		service:        service,
		workerSignaler: workerSignaler,
	}
}
