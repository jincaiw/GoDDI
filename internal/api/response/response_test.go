package response

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func decode(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not JSON: %q (%v)", rec.Body.String(), err)
	}
	return body
}

func assertCommonHeaders(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int) {
	t.Helper()
	if rec.Code != wantStatus {
		t.Fatalf("status = %d, want %d", rec.Code, wantStatus)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
}

// Every response helper must agree on the wire contract: HTTP status, JSON
// code/message, and the security headers. A handler that calls the wrong
// helper otherwise fails silently for clients that read the JSON `code`.
func TestSuccessHelpers(t *testing.T) {
	for _, tc := range []struct {
		name    string
		call    func(http.ResponseWriter)
		status  int
		message string
	}{
		{"OK", func(w http.ResponseWriter) { OK(w, map[string]int{"n": 1}) }, http.StatusOK, "success"},
		{"OKWithMessage", func(w http.ResponseWriter) { OKWithMessage(w, "custom", nil) }, http.StatusOK, "custom"},
		{"Created", func(w http.ResponseWriter) { Created(w, "id") }, http.StatusCreated, "created"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tc.call(rec)
			assertCommonHeaders(t, rec, tc.status)
			body := decode(t, rec)
			if body["code"] != float64(0) || body["message"] != tc.message {
				t.Fatalf("body = %v", body)
			}
		})
	}
}

func TestErrorHelpers(t *testing.T) {
	for _, tc := range []struct {
		name   string
		call   func(http.ResponseWriter)
		status int
		code   int
	}{
		{"BadRequest", func(w http.ResponseWriter) { BadRequest(w, "bad") }, http.StatusBadRequest, 400},
		{"Unauthorized", func(w http.ResponseWriter) { Unauthorized(w, "no") }, http.StatusUnauthorized, 401},
		{"Forbidden", func(w http.ResponseWriter) { Forbidden(w, "no") }, http.StatusForbidden, 403},
		{"NotFound", func(w http.ResponseWriter) { NotFound(w, "gone") }, http.StatusNotFound, 404},
		{"Conflict", func(w http.ResponseWriter) { Conflict(w, "dup") }, http.StatusConflict, 409},
		{"PreconditionFailed", func(w http.ResponseWriter) { PreconditionFailed(w, "stale") }, http.StatusPreconditionFailed, 412},
		{"TooManyRequests", func(w http.ResponseWriter) { TooManyRequests(w, "slow") }, http.StatusTooManyRequests, 429},
		{"InternalError", func(w http.ResponseWriter) { InternalError(w, "oops") }, http.StatusInternalServerError, 500},
		{"ServiceUnavailable", func(w http.ResponseWriter) { ServiceUnavailable(w, "down", nil) }, http.StatusServiceUnavailable, 503},
		{"NotImplemented", func(w http.ResponseWriter) { NotImplemented(w, "reserved") }, http.StatusNotImplemented, 501},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tc.call(rec)
			assertCommonHeaders(t, rec, tc.status)
			body := decode(t, rec)
			if body["code"] != float64(tc.code) || body["message"] == "" {
				t.Fatalf("body = %v", body)
			}
			if _, ok := body["data"]; ok {
				t.Fatalf("error response unexpectedly carries data: %v", body)
			}
		})
	}
}

// A payload-carrying rejection must keep the payload: bulk operations depend
// on it to report per-row failures.
func TestErrorHelpersWithData(t *testing.T) {
	for _, tc := range []struct {
		name   string
		call   func(http.ResponseWriter)
		status int
	}{
		{"BadRequestWithData", func(w http.ResponseWriter) { BadRequestWithData(w, "bad", []string{"row1"}) }, http.StatusBadRequest},
		{"ConflictWithData", func(w http.ResponseWriter) { ConflictWithData(w, "dup", []string{"row2"}) }, http.StatusConflict},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			tc.call(rec)
			assertCommonHeaders(t, rec, tc.status)
			body := decode(t, rec)
			if _, ok := body["data"]; !ok {
				t.Fatalf("payload-carrying error lost its data: %v", body)
			}
		})
	}
}

func TestOKPaginated(t *testing.T) {
	for _, tc := range []struct {
		name                      string
		total                     int64
		page, pageSize, wantPages int
	}{
		{"exact multiple", 40, 2, 20, 2},
		{"remainder rounds up", 41, 1, 20, 3},
		{"zero total", 0, 1, 20, 0},
		{"pageSize clamped to 1", 5, 1, 0, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			OKPaginated(rec, []string{}, tc.total, tc.page, tc.pageSize)
			assertCommonHeaders(t, rec, http.StatusOK)
			body := decode(t, rec)
			meta, ok := body["meta"].(map[string]interface{})
			if !ok {
				t.Fatalf("missing meta: %v", body)
			}
			if meta["total"] != float64(tc.total) || meta["page"] != float64(tc.page) || meta["pages"] != float64(tc.wantPages) {
				t.Fatalf("meta = %v", meta)
			}
			if want := float64(1); tc.pageSize >= 1 {
				if meta["page_size"] != float64(tc.pageSize) {
					t.Fatalf("page_size = %v, want %v", meta["page_size"], tc.pageSize)
				}
			} else if meta["page_size"] != want {
				t.Fatalf("page_size = %v, want clamped 1", meta["page_size"])
			}
		})
	}
}

// A nil (including typed nil) slice must render as [] rather than null: the
// frontend RemoteTable reads data as an array.
func TestOKPaginatedNilDataIsEmptyArray(t *testing.T) {
	var typedNil []string
	for _, data := range []interface{}{nil, typedNil} {
		rec := httptest.NewRecorder()
		OKPaginated(rec, data, 0, 1, 20)
		if !strings.Contains(rec.Body.String(), `"data":[]`) {
			t.Fatalf("nil data rendered as null: %s", rec.Body.String())
		}
	}
}

func TestParsePagination(t *testing.T) {
	for _, tc := range []struct {
		name, query        string
		wantPage, wantSize int
	}{
		{"defaults", "", 1, 20},
		{"valid", "page=3&page_size=50", 3, 50},
		{"invalid coerced", "page=abc&page_size=-1", 1, 20},
		{"zero page", "page=0", 1, 20},
		{"clamped size", "page_size=1000", 1, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/?"+tc.query, nil)
			page, size := ParsePagination(r)
			if page != tc.wantPage || size != tc.wantSize {
				t.Fatalf("page=%d size=%d, want %d/%d", page, size, tc.wantPage, tc.wantSize)
			}
		})
	}
}

// Internal errors and sanitized errors must never echo the underlying error
// text (schema names, paths) to the client.
func TestInternalErrorsDoNotLeakCause(t *testing.T) {
	rec := httptest.NewRecorder()
	InternalErrorWithLog(rec, "generic failure", errors.New("SECRET sqlite column passwd_xyz /var/lib/goddi/db"))
	assertCommonHeaders(t, rec, http.StatusInternalServerError)
	if strings.Contains(rec.Body.String(), "SECRET") || strings.Contains(rec.Body.String(), "passwd_xyz") {
		t.Fatalf("internal error leaked its cause: %s", rec.Body.String())
	}

	r := httptest.NewRequest("GET", "/", nil)
	got := SanitizeError(r, errors.New("SECRET /var/lib/goddi/db"), "safe text")
	if got != "safe text" || strings.Contains(got, "SECRET") {
		t.Fatalf("SanitizeError returned %q", got)
	}
}
