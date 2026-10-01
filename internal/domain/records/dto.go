package records

import "time"

// Response представляет единый внешний ответ со статусом и сообщением.
//   - Status: состояние ресурса, ответа или фильтра выборки.
//   - Message: сообщение чата или безопасный текст ответа согласно указанному типу.
type Response struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// StartRequest передаёт параметры запуска задачи записи.
//   - ConferenceID: идентификатор конференции, ограничивающий область операции.
//   - RequestedBy: значение RequestedBy типа *string, используемое согласно назначению этой операции.
//   - Quality: поддерживаемый профиль качества видео.
//   - QualityMode: значение QualityMode типа string, используемое согласно назначению этой операции.
//   - MinQuality: значение MinQuality типа string, используемое согласно назначению этой операции.
//   - SegmentDurationSec: плановая длительность сегмента записи в секундах.
type StartRequest struct {
	ConferenceID       string  `json:"conferenceId"`
	RequestedBy        *string `json:"requestedBy"`
	Quality            string  `json:"quality"`
	QualityMode        string  `json:"qualityMode"`
	MinQuality         string  `json:"minQuality"`
	SegmentDurationSec int     `json:"segmentDurationSec"`
}

// StartResponse возвращает идентификатор созданной записи и настройки её WebRTC-входа.
//   - Status: состояние ресурса, ответа или фильтра выборки.
//   - Message: сообщение чата или безопасный текст ответа согласно указанному типу.
//   - RecordID: внешний UUID задачи записи.
//   - ConferenceID: идентификатор конференции, ограничивающий область операции.
//   - WebRTC: значение WebRTC типа WebRTCInfo, используемое согласно назначению этой операции.
type StartResponse struct {
	Status       string     `json:"status"`
	Message      string     `json:"message"`
	RecordID     string     `json:"recordId"`
	ConferenceID string     `json:"conferenceId"`
	WebRTC       WebRTCInfo `json:"webrtc"`
}

// EndRequest передаёт идентификатор записи и причину остановки.
//   - RecordID: внешний UUID задачи записи.
//   - Reason: причина завершения, отказа или изменения состояния.
type EndRequest struct {
	RecordID string `json:"recordId"`
	Reason   string `json:"reason"`
}

// Command задаёт согласованное представление данных «Command» для управлении задачами записи и её артефактами.
//   - Type: значение Type типа string, используемое согласно назначению этой операции.
//   - RecordID: внешний UUID задачи записи.
//   - Reason: причина завершения, отказа или изменения состояния.
//   - SegmentDurationSec: плановая длительность сегмента записи в секундах.
type Command struct {
	Type               string `json:"type"`
	RecordID           string `json:"recordId"`
	Reason             string `json:"reason,omitempty"`
	SegmentDurationSec int    `json:"segmentDurationSec,omitempty"`
}

// WebRTCInfo возвращает адрес обмена SDP, ICE-серверы и профиль видеозахвата записи.
//   - OfferURL: значение OfferURL типа string, используемое согласно назначению этой операции.
//   - ICEServers: набор значений ICEServers для последовательной или пакетной обработки.
//   - Video: значение Video типа VideoSettings, используемое согласно назначению этой операции.
type WebRTCInfo struct {
	OfferURL   string        `json:"offerUrl"`
	ICEServers []ICEServer   `json:"iceServers"`
	Video      VideoSettings `json:"video"`
}

// ICEServer описывает адреса и при необходимости учётные данные одного ICE-сервера.
//   - URLs: набор значений URLs для последовательной или пакетной обработки.
type ICEServer struct {
	URLs []string `json:"urls"`
}

// WebRTCOfferRequest передаёт SDP-предложение клиента при установке входа записи.
//   - Type: значение Type типа string, используемое согласно назначению этой операции.
//   - SDP: значение SDP типа string, используемое согласно назначению этой операции.
type WebRTCOfferRequest struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}

// WebRTCAnswerResponse возвращает SDP-ответ приёмника записи.
//   - Type: значение Type типа string, используемое согласно назначению этой операции.
//   - SDP: значение SDP типа string, используемое согласно назначению этой операции.
type WebRTCAnswerResponse struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}

// RecordDetails объединяет запись и связанные файлы, сегменты и события для прикладного чтения.
//   - Record: задача записи с её сохранённым состоянием.
//   - Files: набор файлов или артефактов для обработки.
//   - Segments: доступные сегменты записи для итоговой сборки.
//   - Events: получатель или издатель событий прикладного сценария.
type RecordDetails struct {
	Record   Record
	Files    []RecordFile
	Segments []RecordSegment
	Events   []RecordEvent
}

// RecordCard возвращает карточку записи с разрешёнными ссылками и связанными артефактами.
//   - Record: встроенный тип, добавляющий свой контракт или данные.
//   - Files: набор файлов или артефактов для обработки.
//   - Segments: доступные сегменты записи для итоговой сборки.
//   - Events: получатель или издатель событий прикладного сценария.
type RecordCard struct {
	Record
	Files    []RecordFileView    `json:"files"`
	Segments []RecordSegmentView `json:"segments"`
	Events   []RecordEvent       `json:"events,omitempty"`
}

// RecordFileView добавляет к метаданным файла временную ссылку разрешённого чтения.
//   - RecordFile: встроенный тип, добавляющий свой контракт или данные.
//   - URL: значение URL типа string, используемое согласно назначению этой операции.
type RecordFileView struct {
	RecordFile
	URL string `json:"url,omitempty"`
}

// RecordSegmentView добавляет к метаданным сегмента бакет и временную ссылку чтения.
//   - RecordSegment: встроенный тип, добавляющий свой контракт или данные.
//   - Bucket: имя бакета объектного хранилища.
//   - URL: значение URL типа string, используемое согласно назначению этой операции.
type RecordSegmentView struct {
	RecordSegment
	Bucket string `json:"bucket,omitempty"`
	URL    string `json:"url,omitempty"`
}

// ConferenceRecordSummary собирает количество и краткие карточки записей одной конференции.
//   - ConferenceID: идентификатор конференции, ограничивающий область операции.
//   - RecordsCount: значение RecordsCount типа int64, используемое согласно назначению этой операции.
//   - Records: набор задач записи или их карточек.
type ConferenceRecordSummary struct {
	ConferenceID string                 `json:"conferenceId"`
	RecordsCount int64                  `json:"recordsCount"`
	Records      []ConferenceRecordItem `json:"records"`
}

// ConferenceRecordItem передаёт краткие метаданные записи и ссылки на итоговый файл и превью.
//   - RecordID: внешний UUID задачи записи.
//   - Status: состояние ресурса, ответа или фильтра выборки.
//   - FinalURL: значение FinalURL типа string, используемое согласно назначению этой операции.
//   - PreviewURL: значение PreviewURL типа string, используемое согласно назначению этой операции.
//   - DurationSec: длительность в секундах.
//   - StartedAt: момент начала обработки или записи.
//   - StoppedAt: временная отметка StoppedAt; указатель допускает отсутствие значения.
//   - EndedAt: время завершения записи или физической сессии.
//   - CreatedAt: время создания значения.
type ConferenceRecordItem struct {
	RecordID    string     `json:"recordId"`
	Status      string     `json:"status"`
	FinalURL    string     `json:"finalUrl,omitempty"`
	PreviewURL  string     `json:"previewUrl,omitempty"`
	DurationSec *int       `json:"durationSec,omitempty"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	StoppedAt   *time.Time `json:"stoppedAt,omitempty"`
	EndedAt     *time.Time `json:"endedAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
}
