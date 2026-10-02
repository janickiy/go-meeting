package records

import "github.com/google/uuid"

// ValidateStartRequest проверяет тело старта записи.
// @args
// - request: DTO из HTTP JSON.
// @return текст ошибки для failed response или пустую строку.
func ValidateStartRequest(request StartRequest) string {
	if _, err := uuid.Parse(request.ConferenceID); err != nil {
		return "conferenceId must be valid UUID"
	}
	if request.RequestedBy != nil {
		if _, err := uuid.Parse(*request.RequestedBy); err != nil {
			return "requestedBy must be valid UUID"
		}
	}
	if request.QualityMode == "" {
		return "qualityMode is required"
	}
	if request.QualityMode != "auto" && request.QualityMode != "manual" {
		return "qualityMode must be auto or manual"
	}
	if request.Quality != "" && !IsSupportedVideoQuality(request.Quality) {
		return "quality must be 360p, 480p, 720p or 1080p"
	}
	if request.SegmentDurationSec < 0 {
		return "segmentDurationSec must be positive"
	}

	return ""
}

// ValidateEndRequest проверяет тело остановки записи.
// @args
// - request: DTO из HTTP JSON.
// @return текст ошибки для failed response или пустую строку.
func ValidateEndRequest(request EndRequest) string {
	if _, err := uuid.Parse(request.RecordID); err != nil {
		return "recordId must be valid UUID"
	}
	if request.Reason == "" {
		return "reason is required"
	}

	return ""
}

// ValidateConferenceIDs проверяет список UUID конференций.
// @args
// - conferenceIDs: список conferenceId из query-параметров.
// @return текст ошибки для failed response или пустую строку.
func ValidateConferenceIDs(conferenceIDs []string) string {
	if len(conferenceIDs) == 0 {
		return "conferenceIds is required"
	}
	for _, conferenceID := range conferenceIDs {
		if _, err := uuid.Parse(conferenceID); err != nil {
			return "conferenceIds must contain valid UUID values"
		}
	}

	return ""
}

// ValidateRecordStatusFilter проверяет optional status-фильтр списка/агрегаций.
// @args
// - status: статус записи из query-параметра.
// @return текст ошибки для failed response или пустую строку.
func ValidateRecordStatusFilter(status string) string {
	if status == "" {
		return ""
	}
	if IsSupportedRecordStatus(status) {
		return ""
	}

	return "status must be starting, recording, stopping, finalizing, uploading, ready, partial_ready or failed"
}

// IsSupportedRecordStatus проверяет, входит ли status в известные статусы записи.
// @args
// - status: статус записи.
// @return true, если status поддерживается.
func IsSupportedRecordStatus(status string) bool {
	switch status {
	case StatusStarting,
		StatusRecording,
		StatusStopping,
		StatusFinalizing,
		StatusUploading,
		StatusReady,
		StatusPartialReady,
		StatusFailed:
		return true
	default:
		return false
	}
}
