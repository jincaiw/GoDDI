package response

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"reflect"
	"strconv"
)

// Response is the standard API response structure.
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// PaginatedResponse is a standard paginated API response.
type PaginatedResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
	Meta    PageMeta    `json:"meta,omitempty"`
}

// PageMeta contains pagination metadata.
type PageMeta struct {
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Pages    int   `json:"pages"`
}

// OK sends a successful response with data.
func OK(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusOK, Response{
		Code:    0,
		Message: "success",
		Data:    data,
	})
}

// OKWithMessage sends a successful response with a custom message.
func OKWithMessage(w http.ResponseWriter, message string, data interface{}) {
	writeJSON(w, http.StatusOK, Response{
		Code:    0,
		Message: message,
		Data:    data,
	})
}

// Created sends a 201 Created response.
func Created(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusCreated, Response{
		Code:    0,
		Message: "created",
		Data:    data,
	})
}

// OKPaginated sends a paginated response.
// Nil slices in data are converted to empty slices for JSON output.
func OKPaginated(w http.ResponseWriter, data interface{}, total int64, page, pageSize int) {
	if pageSize < 1 {
		pageSize = 1
	}
	pages := int(total) / pageSize
	if int(total)%pageSize > 0 {
		pages++
	}
	// Guard against nil slice being marshalled as null.
	data = ensureNotNull(data)
	writeJSON(w, http.StatusOK, PaginatedResponse{
		Code:    0,
		Message: "success",
		Data:    data,
		Meta: PageMeta{
			Total:    total,
			Page:     page,
			PageSize: pageSize,
			Pages:    pages,
		},
	})
}

// BadRequest sends a 400 Bad Request response.
func BadRequest(w http.ResponseWriter, message string) {
	writeJSON(w, http.StatusBadRequest, Response{
		Code:    400,
		Message: message,
	})
}

// BadRequestWithData sends a 400 that carries a payload.
//
// A rejected bulk request is refused for reasons that are per row, and a caller
// that only gets a sentence has to guess which rows and why. Passing the report
// back lets the client show the same table a successful run would have shown.
func BadRequestWithData(w http.ResponseWriter, message string, data interface{}) {
	writeJSON(w, http.StatusBadRequest, Response{
		Code:    400,
		Message: message,
		Data:    data,
	})
}

// Unauthorized sends a 401 Unauthorized response.
func Unauthorized(w http.ResponseWriter, message string) {
	writeJSON(w, http.StatusUnauthorized, Response{
		Code:    401,
		Message: message,
	})
}

// Forbidden sends a 403 Forbidden response.
func Forbidden(w http.ResponseWriter, message string) {
	writeJSON(w, http.StatusForbidden, Response{
		Code:    403,
		Message: message,
	})
}

// NotFound sends a 404 Not Found response.
func NotFound(w http.ResponseWriter, message string) {
	writeJSON(w, http.StatusNotFound, Response{
		Code:    404,
		Message: message,
	})
}

// Conflict sends a 409 Conflict response.
func Conflict(w http.ResponseWriter, message string) {
	writeJSON(w, http.StatusConflict, Response{
		Code:    409,
		Message: message,
	})
}

// TooManyRequests sends a 429 Too Many Requests response.
func TooManyRequests(w http.ResponseWriter, message string) {
	writeJSON(w, http.StatusTooManyRequests, Response{
		Code:    429,
		Message: message,
	})
}

// InternalError sends a 500 Internal Server Error response.
func InternalError(w http.ResponseWriter, message string) {
	writeJSON(w, http.StatusInternalServerError, Response{
		Code:    500,
		Message: message,
	})
}

// InternalErrorWithLog reports a generic message to the client while keeping
// the underlying error server-side. Internal errors frequently carry schema
// names, column names and absolute filesystem paths; echoing them hands an
// attacker free reconnaissance about the deployment.
func InternalErrorWithLog(w http.ResponseWriter, message string, err error) {
	if err != nil {
		slog.Error("api: internal error", "message", message, "error", err)
	}
	writeJSON(w, http.StatusInternalServerError, Response{
		Code:    500,
		Message: message,
	})
}

// ServiceUnavailable sends a 503 Service Unavailable response.
func ServiceUnavailable(w http.ResponseWriter, message string, data interface{}) {
	writeJSON(w, http.StatusServiceUnavailable, Response{
		Code:    503,
		Message: message,
		Data:    data,
	})
}

// NotImplemented sends a 501 Not Implemented response.
func NotImplemented(w http.ResponseWriter, message string) {
	writeJSON(w, http.StatusNotImplemented, Response{
		Code:    501,
		Message: message,
	})
}

// SanitizeError returns a safe error message for client-facing responses
// while logging the original (potentially sensitive) error for operators.
// Use this whenever the underlying error may contain DB-internal details
// (SQL errors, file paths, etc.) that should not be exposed to API users.
//
// The first return value is the message to return to the client (always the
// safe text), the second is the original error which has already been
// written to the structured log via slog.
func SanitizeError(r *http.Request, err error, safeClientMessage string) string {
	if err == nil {
		return safeClientMessage
	}
	slog.ErrorContext(r.Context(), "handler error (sanitised in response)",
		"path", r.URL.Path,
		"method", r.Method,
		"client_msg", safeClientMessage,
		"error", err,
	)
	return safeClientMessage
}

// ensureNotNull converts nil slices (including typed nil slices) to empty
// slices so JSON encoding renders them as [] instead of null.
func ensureNotNull(v interface{}) interface{} {
	if v == nil {
		return []interface{}{}
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice && rv.IsNil() {
		return []interface{}{}
	}
	return v
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, statusCode int, v interface{}) {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(v); err != nil {
		// Always set safe response headers before the body, so a client
		// cannot MIME-sniff a generic error page into something dangerous.
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"code":500,"message":"internal server error"}`))
		return
	}
	// X-Content-Type-Options: nosniff tells browsers not to MIME-sniff the
	// response body. This is a defence-in-depth header: even though we
	// explicitly set Content-Type to application/json, an intermediate proxy
	// or a buggy client library could otherwise treat the body as HTML.
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(statusCode)
	w.Write(buf.Bytes())
}

// ParsePagination extracts page and page_size from query parameters.
//
// Design decision: invalid / missing values are silently coerced to safe
// defaults (page=1, page_size=20) rather than rejected with 400. Reasoning:
//   - Pagination params are non-sensitive; rejecting a typo would just
//     confuse the user (and most browsers / SDKs send a 0 page by accident).
//   - The maximum page_size is also clamped to 100 to bound query cost.
//   - If stricter validation is required (e.g. for an external API), introduce
//     a ParsePaginationStrict that returns an error and have those endpoints
//     opt in.
func ParsePagination(r *http.Request) (page, pageSize int) {
	page, _ = strconv.Atoi(r.URL.Query().Get("page"))
	pageSize, _ = strconv.Atoi(r.URL.Query().Get("page_size"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	return page, pageSize
}
