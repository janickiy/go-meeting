package media

// EgressRequest передаёт внутренний запрос записи потока; вход защищён авторизацией и арендой владельца комнаты.
//
//	@params
//	 - RequestID: идентификатор запроса сигнализации для сопоставления ответа.
//	 - ConferenceID: идентификатор конференции, ограничивающий область операции.
//	 - RecordingID: идентификатор записи конференции.
//	 - Route: адрес и версия действующего владельца медиа-комнаты.
//	 - SegmentDurationSec: плановая длительность сегмента записи в секундах.
type EgressRequest struct {
	Purpose            string `json:"purpose,omitempty"`
	RequestID          string `json:"requestId"`
	ConferenceID       string `json:"conferenceId"`
	RecordingID        string `json:"recordingId"`
	Route              Route  `json:"route"`
	SegmentDurationSec int    `json:"segmentDurationSec"`
}

// EgressTrack задаёт согласованное представление данных «выход медиа дорожка» для защищённом управлении медиа-комнатой.
//
//	@params
//	 - Track: встроенный тип, добавляющий свой контракт или данные.
//	 - MimeType: заявленный либо проверенный MIME-тип содержимого.
//	 - ClockRate: значение ClockRate типа uint32, используемое согласно назначению этой операции.
//	 - Channels: значение Channels типа uint16, используемое согласно назначению этой операции.
//	 - PayloadType: значение PayloadType типа uint8, используемое согласно назначению этой операции.
//	 - SSRC: значение SSRC типа uint32, используемое согласно назначению этой операции.
type EgressTrack struct {
	Track
	MimeType    string `json:"mimeType"`
	ClockRate   uint32 `json:"clockRate"`
	Channels    uint16 `json:"channels"`
	PayloadType uint8  `json:"payloadType"`
	SSRC        uint32 `json:"ssrc"`
}

// EgressFrame передаёт закодированный RTP-пакет и время захвата по часам воркера.
// @params
//   - Type: значение Type типа string, используемое согласно назначению этой операции.
//   - Sequence: серверный монотонный номер сообщения или команды.
//   - CapturedAt: значение CapturedAt типа int64, используемое согласно назначению этой операции.
//   - Track: медиа-дорожка, которую обрабатывает или подписывает компонент.
//   - TrackID: идентификатор связанного ресурса, заданного параметром TrackID.
//   - RTP: набор значений RTP для последовательной или пакетной обработки.
//   - Code: код приглашения или машинный код результата.
type EgressFrame struct {
	Type       string       `json:"type"`
	Sequence   uint64       `json:"sequence"`
	CapturedAt int64        `json:"capturedAt"`
	Track      *EgressTrack `json:"track,omitempty"`
	TrackID    string       `json:"trackId,omitempty"`
	RTP        []byte       `json:"rtp,omitempty"`
	Code       string       `json:"code,omitempty"`
}

// EgressSubscription задаёт контракт зависимого компонента EgressSubscription в защищённом управлении медиа-комнатой; позволяет заменять реализацию хранилища или транспорта без изменения вызывающего кода.
//
//	@params
//	 - Frames: операция Frames с контрактом, описанным у метода.
//	 - Done: операция Done с контрактом, описанным у метода.
//	 - Err: операция Err с контрактом, описанным у метода.
//	 - Close: операция закрытие с контрактом, описанным у метода.
//	 - Keyframes: операция Keyframes с контрактом, описанным у метода.
//	 - Ping: операция Ping с контрактом, описанным у метода.
type EgressSubscription interface {
	// Frames возвращает канал кадров подписки записи.
	//
	// @return:
	//   - результат 1 (<-chan EgressFrame): канал данных или уведомления о завершении, принадлежащий жизненному циклу компонента.
	Frames() <-chan EgressFrame
	// Done возвращает канал, закрывающийся при завершении жизненного цикла ресурса.
	//
	//
	// @return:
	//   - результат 1 (<-chan struct{}): канал данных или уведомления о завершении, принадлежащий жизненному циклу компонента.
	Done() <-chan struct{}
	// Err возвращает сохранённую причину завершения ресурса, если оно было ошибочным.
	//
	// @return:
	//   - результат 1 (error): ошибка проверки или выполнения; nil означает успешное завершение.
	Err() error
	// Close закрывает принадлежащие компоненту ресурсы и завершает связанный жизненный цикл.
	//
	Close()
	// Keyframes запрашивает ключевые кадры для источников текущей подписки записи.
	//
	Keyframes()
	// Ping проверяет активность ресурса и связь с его владельцем.
	//
	Ping()
}
