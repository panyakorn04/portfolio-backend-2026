package handler

import (
	"net/http"
	"net/url"
	"strings"

	"portfolio-backend/internal/response"
)

const articleCachePrefix = "portfolio:articles:"
const articleCachePattern = articleCachePrefix + "*"

func articleListCacheKey(locale string, limit *int) string {
	limitValue := "all"
	if limit != nil {
		limitValue = itoa(*limit)
	}
	return articleCachePrefix + "list:lang=" + url.QueryEscape(locale) + ":limit=" + limitValue
}

func articleDetailCacheKey(locale, slug string) string {
	return articleCachePrefix + "detail:lang=" + url.QueryEscape(locale) + ":slug=" + url.QueryEscape(strings.TrimSpace(slug))
}

func writeCachedJSON(w http.ResponseWriter, body []byte) {
	w.Header().Set("X-Cache", "HIT")
	responseWriteJSONBytes(w, http.StatusOK, body)
}

func writeFreshJSON(w http.ResponseWriter, body []byte) {
	w.Header().Set("X-Cache", "MISS")
	responseWriteJSONBytes(w, http.StatusOK, body)
}

func responseWriteJSONBytes(w http.ResponseWriter, status int, body []byte) {
	response.WriteJSONBytes(w, status, body)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}
