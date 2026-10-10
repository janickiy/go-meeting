package notifications

import (
	"context"
	"log/slog"
	"time"

	domain "github.com/janickiy/meet-space/internal/domain/notifications"
	realtime "github.com/janickiy/meet-space/internal/domain/realtime"
)

// Repository задаёт контракт зависимого компонента Repository в личных уведомлениях и их фоновой доставке; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
// @params
//   - List: операция список с контрактом, описанным у метода.
//   - Read: операция чтение с контрактом, описанным у метода.
//   - Generate: операция Generate с контрактом, описанным у метода.
//   - Pending: операция Pending с контрактом, описанным у метода.
//   - Published: операция Published с контрактом, описанным у метода.
type Repository interface {
	// List возвращает ограниченный список личных уведомлений пользователя с принятыми в данном слое фильтрами.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор пользователя, для которого выполняется операция.
	//   - аргумент 3 (string): непрозрачная граница продолжения предыдущей страницы.
	//   - аргумент 4 (int): предел количества обрабатываемых элементов.
	//
	// @return:
	//   - результат 1 (domain.Page): страница элементов и метаданные продолжения.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	List(context.Context, string, string, int) (domain.Page, error)
	// Read идемпотентно отмечает принадлежащее пользователю уведомление прочитанным.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор пользователя, для которого выполняется операция.
	//   - аргумент 3 (string): идентификатор обрабатываемого ресурса.
	//
	// @return:
	//   - результат 1 (domain.Notification): значение, подготовленное операцией для вызывающей стороны.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Read(context.Context, string, string) (domain.Notification, error)
	// Generate создаёт постоянные уведомления из расписания и транзакционных заданий, ограничивая порцию обработки.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Generate(context.Context) error
	// Pending возвращает порцию сохранённых уведомлений, ещё не отмеченных как опубликованные.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//
	// @return:
	//   - результат 1 ([]domain.Notification): собранные элементы результата; состав ограничивается параметрами операции.
	//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Pending(context.Context) ([]domain.Notification, error)
	Publishable(context.Context, string) (bool, error)
	Visible(context.Context, string, string) (bool, error)
	// Published отмечает обработку завершённой после публикации либо подавления доставки.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор обрабатываемого ресурса.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Published(context.Context, string) error
}

// Publisher задаёт контракт зависимого компонента Publisher в личных уведомлениях и их фоновой доставке; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
//   - Publish: операция публикация с контрактом, описанным у метода.
type Publisher interface {
	// Publish передаёт сохранённое изменение через транспорт событий или внутренних команд.
	//
	// @args
	//   - аргумент 1 (context.Context): контекст отмены, дедлайна и времени жизни операции.
	//   - аргумент 2 (string): идентификатор пользователя, для которого выполняется операция.
	//   - аргумент 3 (realtime.Envelope): конверт входящего или публикуемого события.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Publish(context.Context, string, realtime.Envelope) error
}

// Service объединяет зависимости прикладного сценария и координирует его операции.
//   - repo: хранилище постоянных данных прикладного сценария.
//   - bus: транспорт публикации и подписки на доверенные события.
type Service struct {
	repo Repository
	bus  Publisher
}

// NewService создаёт и связывает зависимости компонента Service, используемого в личных уведомлениях и их фоновой доставке.
//
// @args
//   - repo (Repository): хранилище постоянных данных прикладного сценария.
//   - bus (Publisher): транспорт публикации и подписки на доверенные события.
//
// @return:
//   - результат 1 (*Service): созданный компонент с переданными зависимостями.
func NewService(repo Repository, bus Publisher) *Service { return &Service{repo: repo, bus: bus} }

// List возвращает ограниченный список личных уведомлений пользователя с принятыми в данном слое фильтрами.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - cursor (string): непрозрачная граница продолжения предыдущей страницы.
//   - limit (int): максимальное число элементов страницы или порции обработки.
//
// @return:
//   - результат 1 (domain.Page): страница элементов и метаданные продолжения.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) List(ctx context.Context, userID, cursor string, limit int) (domain.Page, error) {
	return s.repo.List(ctx, userID, cursor, limit)
}

// Read идемпотентно отмечает принадлежащее пользователю уведомление прочитанным.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - id (string): идентификатор обрабатываемого ресурса.
//
// @return:
//   - результат 1 (domain.Notification): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) Read(ctx context.Context, userID, id string) (domain.Notification, error) {
	item, err := s.repo.Read(ctx, userID, id)
	if err != nil {
		return item, err
	}
	// Состояние прочтения сохраняется до публикации. Опрос и переподключение восстанавливают
	// пропущенные события Redis; чувствительное содержимое не попадает в общую рассылку конференции.
	event := realtime.Event("notification.read", "", map[string]any{"id": item.ID})
	_ = s.bus.Publish(ctx, userID, event)
	return item, nil
}

// Visible authorizes a queued notification again immediately before delivery.
func (s *Service) Visible(ctx context.Context, userID, id string) (bool, error) {
	return s.repo.Visible(ctx, userID, id)
}

// Tick выполняет один цикл создания и доставки уведомлений после фиксации постоянных данных.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (s *Service) Tick(ctx context.Context) error {
	if err := s.repo.Generate(ctx); err != nil {
		return err
	}
	pending, err := s.repo.Pending(ctx)
	if err != nil {
		return err
	}
	for _, item := range pending {
		allowed, err := s.repo.Publishable(ctx, item.ID)
		if err != nil {
			return err
		}
		if allowed {
			event := realtime.Event("notification.created", "", map[string]any{"notification": item})
			event.ID = item.ID // Повторные доставки одного события имеют стабильный идентификатор.
			if err = s.bus.Publish(ctx, item.UserID, event); err != nil {
				return err
			}
		}
		if err = s.repo.Published(ctx, item.ID); err != nil {
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
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		op, cancel := context.WithTimeout(ctx, 4*time.Second)
		err := s.Tick(op)
		cancel()
		if err != nil && ctx.Err() == nil {
			slog.Warn("notification job failed")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
