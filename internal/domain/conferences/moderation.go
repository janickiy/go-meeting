package conferences

import "github.com/janickiy/meet-space/internal/domain/apperrors"

// MediaState описывает заявленную активность микрофона, камеры и экрана участника.
// @params
//   - ConnectionID: идентификатор физического медиа-соединения.
//   - Sequence: серверный монотонный номер сообщения или команды.
//   - MicrophoneEnabled: сохранённый признак включённого микрофона.
//   - CameraEnabled: сохранённый признак включённой камеры.
//   - ScreenSharing: сохранённый признак демонстрации экрана.
type MediaState struct {
	ConnectionID      string `json:"connectionId"`
	Sequence          int64  `json:"sequence"`
	MicrophoneEnabled bool   `json:"microphoneEnabled"`
	CameraEnabled     bool   `json:"cameraEnabled"`
	ScreenSharing     bool   `json:"screenSharing"`
}

// ModerationRequest передаёт действие модерации и его дополнительные параметры.
// @params
//   - Action: действие управления, которое необходимо проверить или исполнить.
//   - Blocked: значение Blocked типа *bool, используемое согласно назначению этой операции.
//   - Role: роль участника, определяющая полномочия.
type ModerationRequest struct {
	Action  string `json:"action"`
	Blocked *bool  `json:"blocked,omitempty"`
	Role    Role   `json:"role,omitempty"`
}

// CanModerate сопоставляет роли и состояния инициатора и цели; защищает владельца и ограничивает действия соорганизатора.
//
// @args
//   - actor (Participant): участник, от имени которого проверяются полномочия действия.
//   - target (Participant): целевой объект, участник или состояние операции.
//   - action (string): действие управления, которое необходимо проверить или исполнить.
//
// @return:
//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
func CanModerate(actor, target Participant, action string) bool {
	switch action {
	case "mute", "camera", "screen", "kick", "role":
	default:
		return false
	}
	if !actor.CanParticipate() || actor.ConferenceID != target.ConferenceID || target.Role == Owner || actor.ID == target.ID {
		return false
	}
	// Ожидающие участники видны только в зале ожидания и обрабатываются через
	// admit/reject. Общие события модерации доступны допущенным участникам комнаты.
	// Повтор уже выполненного удаления участника остаётся идемпотентным.
	if !target.CanReadHistory() && !(action == "kick" && target.Status == Kicked) {
		return false
	}
	if actor.Role == Owner {
		return true
	}
	return actor.Role == CoHost && target.Role == ParticipantRole && (action == "mute" || action == "screen" || action == "kick")
}

// Validate проверяет ограничения и согласованность полей текущего значения перед его использованием.
//
// @return:
//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
func (r ModerationRequest) Validate() error {
	switch r.Action {
	case "mute", "camera", "screen":
		if r.Blocked == nil || r.Role != "" {
			return apperrors.New(apperrors.ErrInvalidInput, "blocked is required for media moderation")
		}
	case "kick":
		if r.Blocked != nil || r.Role != "" {
			return apperrors.ErrInvalidInput
		}
	case "role":
		if r.Blocked != nil || (r.Role != CoHost && r.Role != ParticipantRole) {
			return apperrors.New(apperrors.ErrInvalidInput, "role must be co_host or participant")
		}
	default:
		return apperrors.New(apperrors.ErrInvalidInput, "unsupported moderation action")
	}
	return nil
}
