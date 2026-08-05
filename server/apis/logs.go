package apis

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	applogs "nexgestion/server/logs"
)

func readLogs(service *applogs.Service) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		start, err := parseLogTime(r.URL.Query().Get("start"), now.Add(-24*time.Hour))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid start time"})
			return
		}
		end, err := parseLogTime(r.URL.Query().Get("end"), now)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid end time"})
			return
		}
		if end.Before(start) || start.Before(now.Add(-7*24*time.Hour)) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "time range must be within the last seven days"})
			return
		}
		statuses, err := applogs.ParseStatuses(r.URL.Query().Get("status"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		limit := 100
		if value := strings.TrimSpace(r.URL.Query().Get("limit")); value != "" {
			limit, err = strconv.Atoi(value)
			if err != nil || limit < 1 || limit > 1000 {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "limit must be between 1 and 1000"})
				return
			}
		}
		listQuery, err := parseListQuery(r)
		if err != nil {
			writeListQueryError(w, err)
			return
		}
		if r.URL.Query().Get("cursor") != "" && (r.URL.Query().Get("page") != "" || r.URL.Query().Get("page_size") != "") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cursor pagination cannot be combined with page pagination"})
			return
		}
		result, err := service.Read(applogs.Query{
			Start:    start,
			End:      end,
			Statuses: statuses,
			Limit:    limit,
			Cursor:   r.URL.Query().Get("cursor"),
			Page:     listQuery.Page,
			PageSize: listQuery.PageSize,
			Keyword:  listQuery.Keyword,
			Sort:     listQuery.Sort,
			Order:    listQuery.Order,
		})
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, applogs.ErrInvalidStatus) || err.Error() == "invalid cursor" || strings.HasPrefix(err.Error(), "invalid ") {
				status = http.StatusBadRequest
			}
			message := "unable to read logs"
			if status == http.StatusBadRequest {
				message = err.Error()
			}
			writeJSON(w, status, map[string]string{"error": message})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"logs":        result.Logs,
			"next_cursor": result.NextCursor,
			"pagination": map[string]int{
				"page":        result.Page,
				"page_size":   result.PageSize,
				"total":       result.Total,
				"total_pages": result.TotalPages,
			},
			"sort": map[string]string{
				"field": result.Sort,
				"order": result.Order,
			},
			"keyword": result.Keyword,
		})
	}
}

func parseLogTime(value string, fallback time.Time) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	return time.Parse(time.RFC3339, value)
}
