package realtime

// AllowedReaction проверяет, входит ли эмодзи в список разрешённых реакций 👍, 👏, ❤️ и 😂.
//
// @args
//   - emoji (string): одна из разрешённых временных реакций.
//
// @return:
//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
func AllowedReaction(emoji string) bool {
	return emoji == "👍" || emoji == "👏" || emoji == "❤️" || emoji == "😂"
}

// LowPriorityEvent отделяет восстанавливаемые события чата и реакций от критичных событий управления звонком.
//
// @args
//   - kind (string): тип события, ошибки или медиа, определяющий ветку обработки.
//
// @return:
//   - результат 1 (bool): признак выполнения проверяемого условия или изменения состояния.
func LowPriorityEvent(kind string) bool {
	switch kind {
	case "caption.partial", "caption.final", "caption.status", "reaction.created", "chat.message.created", "chat.message.updated", "chat.message.deleted", "chat.read.updated":
		return true
	default:
		return false
	}
}
