package personal

// PeerPresence передаёт подтверждённое присутствие собеседника доступного личного чата.
// Не содержит имя, email, идентификаторы сессий или количество соединений.
// @params
//   - ConversationID: личная переписка, доступ к которой проверен для текущего аккаунта.
//   - PeerID: собеседник, определённый сервером по участникам этой переписки.
//   - Online: наличие действующей учётной WebSocket-сессии по данным хранилища присутствия.
type PeerPresence struct {
	ConversationID string `json:"conversationId"`
	PeerID         string `json:"peerId"`
	Online         bool   `json:"online"`
}
