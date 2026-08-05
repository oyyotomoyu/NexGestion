package apis

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"nexgestion/server/system"
)

func parseListQuery(r *http.Request) (system.ListQuery, error) {
	values := r.URL.Query()
	query := system.ListQuery{
		Page:     system.DefaultPage,
		PageSize: system.DefaultPageSize,
		Keyword:  strings.TrimSpace(values.Get("keyword")),
		Sort:     strings.TrimSpace(values.Get("sort")),
		Order:    strings.TrimSpace(values.Get("order")),
		Filters:  map[string]string{},
	}
	if value := strings.TrimSpace(values.Get("page")); value != "" {
		page, err := strconv.Atoi(value)
		if err != nil {
			return query, system.ErrInvalidListQuery
		}
		query.Page = page
	}
	if value := strings.TrimSpace(values.Get("page_size")); value != "" {
		pageSize, err := strconv.Atoi(value)
		if err != nil {
			return query, system.ErrInvalidListQuery
		}
		query.PageSize = pageSize
	}
	for key, value := range values {
		if len(value) == 0 {
			continue
		}
		switch key {
		case "page", "page_size", "keyword", "sort", "order":
			continue
		default:
			query.Filters[key] = strings.TrimSpace(value[0])
		}
	}
	return query, nil
}

func writeListQueryError(w http.ResponseWriter, err error) bool {
	if errors.Is(err, system.ErrInvalidListQuery) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return true
	}
	return false
}

func listResponse[T any](key string, result system.ListResult[T]) map[string]any {
	return map[string]any{
		key:          result.Items,
		"pagination": result.Pagination,
		"sort":       result.Sort,
		"keyword":    result.Keyword,
	}
}
