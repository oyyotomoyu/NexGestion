package system

import (
	"errors"
	"math"
	"strings"
)

const (
	DefaultPage     = 1
	DefaultPageSize = 20
	MaxPageSize     = 100
)

var ErrInvalidListQuery = errors.New("invalid list query")

type ListQuery struct {
	Page     int
	PageSize int
	Keyword  string
	Sort     string
	Order    string
	Filters  map[string]string
}

type Pagination struct {
	Page       int `json:"page"`
	PageSize   int `json:"page_size"`
	Total      int `json:"total"`
	TotalPages int `json:"total_pages"`
}

type SortResult struct {
	Field string `json:"field"`
	Order string `json:"order"`
}

type ListResult[T any] struct {
	Items      []T
	Pagination Pagination
	Sort       SortResult
	Keyword    string
}

func NormalizeListQuery(query ListQuery, defaultSort, defaultOrder string, allowedSorts map[string]string) (ListQuery, string, error) {
	if query.Page == 0 {
		query.Page = DefaultPage
	}
	if query.PageSize == 0 {
		query.PageSize = DefaultPageSize
	}
	if query.Page < 1 || query.PageSize < 1 || query.PageSize > MaxPageSize {
		return query, "", ErrInvalidListQuery
	}
	query.Keyword = strings.TrimSpace(query.Keyword)
	query.Sort = strings.TrimSpace(query.Sort)
	if query.Sort == "" {
		query.Sort = defaultSort
	}
	sortExpression, ok := allowedSorts[query.Sort]
	if !ok {
		return query, "", ErrInvalidListQuery
	}
	query.Order = strings.ToLower(strings.TrimSpace(query.Order))
	if query.Order == "" {
		query.Order = defaultOrder
	}
	if query.Order != "asc" && query.Order != "desc" {
		return query, "", ErrInvalidListQuery
	}
	return query, sortExpression, nil
}

func NewListResult[T any](items []T, query ListQuery, total int) ListResult[T] {
	totalPages := 0
	if total > 0 {
		totalPages = int(math.Ceil(float64(total) / float64(query.PageSize)))
	}
	return ListResult[T]{
		Items: items,
		Pagination: Pagination{
			Page:       query.Page,
			PageSize:   query.PageSize,
			Total:      total,
			TotalPages: totalPages,
		},
		Sort:    SortResult{Field: query.Sort, Order: query.Order},
		Keyword: query.Keyword,
	}
}

func ListOffset(query ListQuery) int {
	return (query.Page - 1) * query.PageSize
}
