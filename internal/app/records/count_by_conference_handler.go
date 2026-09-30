package recordsapp

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/janickiy/go-recorder/internal/domain/records"
)

// CountByConference возвращает количество записей и краткие карточки записей для переданных conferenceId.
// Параметры:
// - c: Gin context HTTP-запроса.
// Query-параметры:
// - conferenceIds[] или conferenceIds: один или несколько UUID конференций.
// - status: optional фильтр по статусу записи.
// Возвращает: JSON response со списком conferenceId, recordsCount и records[].
func (h *Handler) CountByConference(c *gin.Context) {
	conferenceIDs := conferenceIDsFromQuery(c)
	if message := records.ValidateConferenceIDs(conferenceIDs); message != "" {
		failed(c, http.StatusBadRequest, message)
		return
	}
	status := strings.TrimSpace(c.Query("status"))
	if message := records.ValidateRecordStatusFilter(status); message != "" {
		failed(c, http.StatusBadRequest, message)
		return
	}

	items, err := h.service.CountByConference(c.Request.Context(), conferenceIDs, status)
	if err != nil {
		failed(c, http.StatusInternalServerError, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "items": items})
}

func conferenceIDsFromQuery(c *gin.Context) []string {
	values := make([]string, 0)
	for _, key := range []string{"conferenceIds[]", "conferenceIds"} {
		for _, raw := range c.QueryArray(key) {
			for _, value := range strings.Split(raw, ",") {
				value = strings.TrimSpace(value)
				if value != "" {
					values = append(values, value)
				}
			}
		}
	}

	return uniqueStrings(values)
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}

	return result
}
