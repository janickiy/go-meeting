package postgres

import (
	"context"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/janickiy/go-recorder/internal/domain/apperrors"
	"github.com/janickiy/go-recorder/internal/domain/chat"
	"github.com/janickiy/go-recorder/internal/domain/conferences"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"
)

// ChatRepository реализует постоянное хранение сообщений и вложений конференции через GORM.
//   - db: подключение или текущая транзакция GORM, задающая контекст доступа к базе.
type ChatRepository struct{ db *gorm.DB }

// NewChatRepository создаёт и связывает зависимости компонента ChatRepository, используемого в постоянном чате и приватных вложениях.
//
// @args
//   - db (*gorm.DB): подключение или текущая транзакция GORM, задающая контекст доступа к базе.
//
// @return:
//   - результат 1 (*ChatRepository): созданный компонент с переданными зависимостями.
func NewChatRepository(db *gorm.DB) *ChatRepository {
	// SQL, который GORM подставляет в журналы медленных запросов и ошибок, может содержать
	// приватный текст сообщений или имена файлов. Диагностика приложения выводит только ID событий.
	return &ChatRepository{db: db.Session(&gorm.Session{Logger: logger.Default.LogMode(logger.Silent)})}
}

// authorizeChat проверяет членство и допуск к истории; для изменения чата дополнительно требует активную встречу и joined.
//
// @args
//   - tx (*gorm.DB): подключение или текущая транзакция GORM, задающая контекст доступа к базе.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - locked (bool): указывает, требуется ли чтение с блокировкой строки для согласованного изменения.
//   - write (bool): указывает, необходимо ли проверять право изменения, а не только чтения.
//
// @return:
//   - результат 1 (conferences.Participant): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func authorizeChat(tx *gorm.DB, userID, conferenceID string, locked, write bool) (conferences.Participant, error) {
	c, err := findConference(tx, conferenceID, locked)
	if err != nil {
		return conferences.Participant{}, err
	}
	p, err := findMembership(tx, conferenceID, userID)
	if errors.Is(err, apperrors.ErrNotFound) {
		return p, apperrors.ErrForbidden
	}
	if err != nil {
		return p, err
	}
	if !p.CanReadHistory() {
		return p, apperrors.ErrForbidden
	}
	if write && (c.Status != conferences.Active || !p.CanParticipate()) {
		return p, apperrors.New(apperrors.ErrConflict, "chat is read-only unless the conference is active and you have joined")
	}
	return p, nil
}

// chatCursor кодирует идентификатор конференции и последовательность сообщения в курсор истории.
//
// @args
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - sequence (int64): серверный монотонный номер сообщения или команды.
//
// @return:
//   - результат 1 (string): кодированная граница следующей страницы.
func chatCursor(conferenceID string, sequence int64) string {
	return base64.RawURLEncoding.EncodeToString([]byte(conferenceID + ":" + strconv.FormatInt(sequence, 10)))
}

// parseChatCursor проверяет формат курсора и его принадлежность конференции, затем извлекает номер сообщения.
//
// @args
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - cursor (string): непрозрачная граница продолжения предыдущей страницы.
//
// @return:
//   - результат 1 (int64): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func parseChatCursor(conferenceID, cursor string) (int64, error) {
	if cursor == "" {
		return 0, nil
	}
	if len(cursor) > 128 {
		return 0, apperrors.ErrInvalidInput
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return 0, apperrors.ErrInvalidInput
	}
	parts := strings.Split(string(raw), ":")
	if len(parts) != 2 || parts[0] != conferenceID {
		return 0, apperrors.ErrInvalidInput
	}
	n, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || n < 1 {
		return 0, apperrors.ErrInvalidInput
	}
	return n, nil
}

// messageQuery строит базовый запрос сообщений с именем отправителя из членства конференции.
//
// @args
//   - tx (*gorm.DB): подключение или текущая транзакция GORM, задающая контекст доступа к базе.
//
// @return:
//   - результат 1 (*gorm.DB): значение, подготовленное операцией для вызывающей стороны.
func messageQuery(tx *gorm.DB) *gorm.DB {
	return tx.Model(&chat.Message{}).Select("chat_messages.*, p.display_name AS sender_name").Joins("JOIN conference_participants p ON p.conference_id=chat_messages.conference_id AND p.user_id=chat_messages.sender_user_id")
}

// loadMessage читает одно сообщение внутри конференции и добавляет вложения и краткое представление ответа.
//
// @args
//   - tx (*gorm.DB): подключение или текущая транзакция GORM, задающая контекст доступа к базе.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//
// @return:
//   - результат 1 (chat.Message): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func loadMessage(tx *gorm.DB, conferenceID, id string) (chat.Message, error) {
	var message chat.Message
	err := messageQuery(tx).Where("chat_messages.id=? AND chat_messages.conference_id=?", id, conferenceID).Take(&message).Error
	if err != nil {
		return message, mapNotFound(err)
	}
	rows := []chat.Message{message}
	err = decorateMessages(tx, rows)
	return rows[0], err
}

// decorateMessages пакетно добавляет вложения и ответы к странице сообщений и скрывает содержимое удалённых сообщений.
//
// @args
//   - tx (*gorm.DB): подключение или текущая транзакция GORM, задающая контекст доступа к базе.
//   - items ([]chat.Message): элементы страницы или порции пакетной обработки.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func decorateMessages(tx *gorm.DB, items []chat.Message) error {
	if len(items) == 0 {
		return nil
	}
	ids := make([]string, 0, len(items))
	replyIDs := []string{}
	for i := range items {
		items[i].Attachments = []chat.Attachment{}
		ids = append(ids, items[i].ID)
		if items[i].ReplyTo != nil {
			replyIDs = append(replyIDs, *items[i].ReplyTo)
		}
	}
	var attachments []chat.Attachment
	if err := tx.Where("message_id IN ? AND status='attached'", ids).Order("created_at,id").Find(&attachments).Error; err != nil {
		return err
	}
	byMessage := map[string][]chat.Attachment{}
	for _, a := range attachments {
		byMessage[*a.MessageID] = append(byMessage[*a.MessageID], a)
	}
	replies := map[string]chat.ReplyPreview{}
	if len(replyIDs) > 0 {
		var rows []chat.Message
		if err := messageQuery(tx).Where("chat_messages.conference_id=? AND chat_messages.id IN ?", items[0].ConferenceID, replyIDs).Find(&rows).Error; err != nil {
			return err
		}
		for _, m := range rows {
			replies[m.ID] = chat.ReplyPreview{ID: m.ID, Text: m.Text, SenderName: m.SenderName, Deleted: m.DeletedAt != nil}
		}
	}
	for i := range items {
		if items[i].DeletedAt == nil {
			if files := byMessage[items[i].ID]; files != nil {
				items[i].Attachments = files
			}
		} else {
			items[i].Text = ""
		}
		if items[i].ReplyTo != nil {
			if reply, ok := replies[*items[i].ReplyTo]; ok {
				items[i].ReplyPreview = &reply
			}
		}
	}
	return nil
}

// readState читает границу прочтения и считает чужие неудалённые сообщения после неё.
//
// @args
//   - tx (*gorm.DB): подключение или текущая транзакция GORM, задающая контекст доступа к базе.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//
// @return:
//   - результат 1 (chat.ReadState): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func readState(tx *gorm.DB, userID, conferenceID string) (chat.ReadState, error) {
	var state chat.ReadState
	if err := tx.Table("chat_read_states").Select("last_read_message_id,last_read_sequence").Where("conference_id=? AND user_id=?", conferenceID, userID).Scan(&state).Error; err != nil {
		return state, err
	}
	err := tx.Model(&chat.Message{}).Where("conference_id=? AND sequence>? AND sender_user_id<>? AND deleted_at IS NULL", conferenceID, state.LastReadSequence, userID).Count(&state.UnreadCount).Error
	return state, err
}

// List возвращает ограниченный список сообщений и вложений конференции с принятыми в данном слое фильтрами.
// Операции с базой данных объединяет в транзакцию.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - cursor (string): непрозрачная граница продолжения предыдущей страницы.
//   - limit (int): максимальное число элементов страницы или порции обработки.
//
// @return:
//   - результат 1 (chat.Page): страница элементов и метаданные продолжения.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *ChatRepository) List(ctx context.Context, userID, conferenceID, cursor string, limit int) (chat.Page, error) {
	page := chat.Page{Items: []chat.Message{}}
	before, err := parseChatCursor(conferenceID, cursor)
	if err != nil {
		return page, err
	}
	if limit < 1 || limit > 100 {
		return page, apperrors.New(apperrors.ErrInvalidInput, "limit must be between 1 and 100")
	}
	err = r.db.WithContext(ctx).Transaction( /* Вложенный обработчик выполняет часть операции в текущей транзакции базы данных, сохраняя её общий результат.

		@args
		  - tx (*gorm.DB): подключение или текущая транзакция GORM, задающая контекст доступа к базе.

		@return:
		  - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func(tx *gorm.DB) error {
			if _, err := authorizeChat(tx, userID, conferenceID, false, false); err != nil {
				return err
			}
			query := messageQuery(tx).Where("chat_messages.conference_id=?", conferenceID)
			if before > 0 {
				query = query.Where("sequence<?", before)
			}
			if err := query.Order("sequence DESC").Limit(limit + 1).Find(&page.Items).Error; err != nil {
				return err
			}
			if len(page.Items) > limit {
				page.Items = page.Items[:limit]
				page.NextCursor = chatCursor(conferenceID, page.Items[len(page.Items)-1].Sequence)
			}
			for i, j := 0, len(page.Items)-1; i < j; i, j = i+1, j-1 {
				page.Items[i], page.Items[j] = page.Items[j], page.Items[i]
			}
			if err := decorateMessages(tx, page.Items); err != nil {
				return err
			}
			state, err := readState(tx, userID, conferenceID)
			page.UnreadCount = state.UnreadCount
			page.LastReadMessageID = state.LastReadMessageID
			return err
		})
	return page, err
}

// Send сохраняет сообщение чата с проверкой доступа, ответа и вложений; ключ запроса защищает повторную отправку от дубля.
// Операции с базой данных объединяет в транзакцию.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - request (chat.SendRequest): входные параметры соответствующего прикладного запроса.
//   - fingerprint (string): отпечаток нормализованного запроса для сравнения повторных отправок.
//
// @return:
//   - результат 1 (chat.Message): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (bool): признак выполнения проверяемого условия или изменения состояния.
//   - результат 3 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *ChatRepository) Send(ctx context.Context, userID, conferenceID string, request chat.SendRequest, fingerprint string) (chat.Message, bool, error) {
	var message chat.Message
	created := false
	err := r.db.WithContext(ctx).Transaction( /* Вложенный обработчик выполняет часть операции в текущей транзакции базы данных, сохраняя её общий результат.

		@args
		  - tx (*gorm.DB): подключение или текущая транзакция GORM, задающая контекст доступа к базе.

		@return:
		  - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func(tx *gorm.DB) error {
			if _, err := authorizeChat(tx, userID, conferenceID, true, false); err != nil {
				return err
			}
			err := tx.Where("conference_id=? AND sender_user_id=? AND client_request_id=?", conferenceID, userID, request.ClientRequestID).Take(&message).Error
			if err == nil {
				if message.RequestFingerprint != fingerprint {
					return apperrors.New(apperrors.ErrConflict, "clientRequestId was already used for a different message")
				}
				return nil
			}
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if _, err := authorizeChat(tx, userID, conferenceID, false, true); err != nil {
				return err
			}
			var replyTo *string
			if request.ReplyTo != "" {
				var reply chat.Message
				if err := tx.Where("id=? AND conference_id=? AND deleted_at IS NULL", request.ReplyTo, conferenceID).Take(&reply).Error; err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						return apperrors.New(apperrors.ErrInvalidInput, "reply target is unavailable in this conference")
					}
					return err
				}
				replyTo = &request.ReplyTo
			}
			var attachments []chat.Attachment
			if len(request.AttachmentIDs) > 0 {
				if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id IN ?", request.AttachmentIDs).Find(&attachments).Error; err != nil {
					return err
				}
				if len(attachments) != len(request.AttachmentIDs) {
					return apperrors.New(apperrors.ErrInvalidInput, "attachment is unavailable")
				}
				for _, a := range attachments {
					if a.ConferenceID != conferenceID || a.OwnerID != userID || a.Status != "ready" || !a.ExpiresAt.After(time.Now()) {
						return apperrors.New(apperrors.ErrConflict, "attachment is unavailable or expired")
					}
				}
			}
			message = chat.Message{ID: uuid.NewString(), ConferenceID: conferenceID, SenderID: userID, ClientRequestID: request.ClientRequestID, RequestFingerprint: fingerprint, Text: request.Text, ReplyTo: replyTo, Version: 1}
			if err := tx.Create(&message).Error; err != nil {
				return err
			}
			if len(attachments) > 0 {
				if err := tx.Model(&chat.Attachment{}).Where("id IN ?", request.AttachmentIDs).Updates(map[string]any{"status": "attached", "message_id": message.ID, "updated_at": gorm.Expr("clock_timestamp()")}).Error; err != nil {
					return err
				}
			}
			created = true
			return nil
		})
	if err != nil {
		return chat.Message{}, false, err
	}
	message, err = loadMessage(r.db.WithContext(ctx), conferenceID, message.ID)
	return message, created, err
}

// Edit изменяет текст собственного неудалённого сообщения с проверкой доступа и состояния встречи.
// Операции с базой данных объединяет в транзакцию.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//   - text (string): обычный текст сообщения, подлежащий проверке или обработке.
//
// @return:
//   - результат 1 (chat.Message): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *ChatRepository) Edit(ctx context.Context, userID, conferenceID, id, text string) (chat.Message, error) {
	err := r.db.WithContext(ctx).Transaction( /* Вложенный обработчик выполняет часть операции в текущей транзакции базы данных, сохраняя её общий результат.

		@args
		  - tx (*gorm.DB): подключение или текущая транзакция GORM, задающая контекст доступа к базе.

		@return:
		  - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func(tx *gorm.DB) error {
			if _, err := authorizeChat(tx, userID, conferenceID, true, true); err != nil {
				return err
			}
			var message chat.Message
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND conference_id=?", id, conferenceID).Take(&message).Error; err != nil {
				return mapNotFound(err)
			}
			if message.SenderID != userID {
				return apperrors.ErrForbidden
			}
			if message.DeletedAt != nil {
				return apperrors.New(apperrors.ErrConflict, "message was deleted")
			}
			if text == "" {
				var count int64
				if err := tx.Model(&chat.Attachment{}).Where("message_id=? AND status='attached'", id).Count(&count).Error; err != nil {
					return err
				}
				if count == 0 {
					return apperrors.New(apperrors.ErrInvalidInput, "message cannot be empty")
				}
			}
			return tx.Model(&chat.Message{}).Where("id=?", id).Updates(map[string]any{"text": text, "version": gorm.Expr("version+1"), "updated_at": gorm.Expr("clock_timestamp()")}).Error
		})
	if err != nil {
		return chat.Message{}, err
	}
	return loadMessage(r.db.WithContext(ctx), conferenceID, id)
}

// Delete мягко удаляет доступное сообщение, проверяя автора или полномочия модератора и сохраняя историю.
// Операции с базой данных объединяет в транзакцию.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - id (string): идентификатор обрабатываемого ресурса.
//
// @return:
//   - результат 1 (chat.Message): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *ChatRepository) Delete(ctx context.Context, userID, conferenceID, id string) (chat.Message, error) {
	err := r.db.WithContext(ctx).Transaction( /* Вложенный обработчик выполняет часть операции в текущей транзакции базы данных, сохраняя её общий результат.

		@args
		  - tx (*gorm.DB): подключение или текущая транзакция GORM, задающая контекст доступа к базе.

		@return:
		  - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func(tx *gorm.DB) error {
			p, err := authorizeChat(tx, userID, conferenceID, true, true)
			if err != nil {
				return err
			}
			var message chat.Message
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id=? AND conference_id=?", id, conferenceID).Take(&message).Error; err != nil {
				return mapNotFound(err)
			}
			if message.SenderID != userID && p.Role != conferences.Owner && p.Role != conferences.CoHost {
				return apperrors.ErrForbidden
			}
			if message.DeletedAt != nil {
				return nil
			}
			return tx.Model(&chat.Message{}).Where("id=?", id).Updates(map[string]any{"text": "", "deleted_at": gorm.Expr("clock_timestamp()"), "updated_at": gorm.Expr("clock_timestamp()"), "version": gorm.Expr("version+1")}).Error
		})
	if err != nil {
		return chat.Message{}, err
	}
	return loadMessage(r.db.WithContext(ctx), conferenceID, id)
}

// ReadState возвращает сохранённую границу прочтения и число доступных непрочитанных сообщений.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//
// @return:
//   - результат 1 (chat.ReadState): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *ChatRepository) ReadState(ctx context.Context, userID, conferenceID string) (chat.ReadState, error) {
	tx := r.db.WithContext(ctx)
	p, err := authorizeChat(tx, userID, conferenceID, false, false)
	if err != nil {
		return chat.ReadState{}, err
	}
	state, err := readState(tx, userID, conferenceID)
	state.ParticipantID = p.ID
	return state, err
}

// MarkRead продвигает сохранённое состояние прочтения; повторные и запоздалые запросы не должны уменьшать курсор.
// Операции с базой данных объединяет в транзакцию.
//
// @args
//   - ctx (context.Context): контекст отмены, дедлайна и времени жизни операции.
//   - userID (string): идентификатор пользователя, для которого выполняется операция.
//   - conferenceID (string): идентификатор конференции, ограничивающий область операции.
//   - messageID (string): идентификатор сообщения внутри конференции.
//
// @return:
//   - результат 1 (chat.ReadState): значение, подготовленное операцией для вызывающей стороны.
//   - результат 2 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r *ChatRepository) MarkRead(ctx context.Context, userID, conferenceID, messageID string) (chat.ReadState, error) {
	err := r.db.WithContext(ctx).Transaction( /* Вложенный обработчик выполняет часть операции в текущей транзакции базы данных, сохраняя её общий результат.

		@args
		  - tx (*gorm.DB): подключение или текущая транзакция GORM, задающая контекст доступа к базе.

		@return:
		  - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение. */func(tx *gorm.DB) error {
			if _, err := authorizeChat(tx, userID, conferenceID, true, false); err != nil {
				return err
			}
			var message chat.Message
			if err := tx.Select("id,sequence").Where("id=? AND conference_id=?", messageID, conferenceID).Take(&message).Error; err != nil {
				return mapNotFound(err)
			}
			return tx.Exec(`INSERT INTO chat_read_states(conference_id,user_id,last_read_message_id,last_read_sequence) VALUES(?,?,?,?) ON CONFLICT(conference_id,user_id) DO UPDATE SET last_read_message_id=EXCLUDED.last_read_message_id,last_read_sequence=EXCLUDED.last_read_sequence,updated_at=clock_timestamp() WHERE chat_read_states.last_read_sequence<EXCLUDED.last_read_sequence`, conferenceID, userID, messageID, message.Sequence).Error
		})
	if err != nil {
		return chat.ReadState{}, err
	}
	return r.ReadState(ctx, userID, conferenceID)
}
