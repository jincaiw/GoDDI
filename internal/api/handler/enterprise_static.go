package handler

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jasonwa/goddi/internal/enterprise/ipamaddr"
)

const enterpriseBodyLimit = 1 << 20

func isEnterpriseStatic(r *http.Request) bool {
	p := strings.Split(r.URL.Path, "/")
	if len(p) == 11 && p[10] == "" {
		p = p[:10]
	}
	return len(p) == 10 && p[1] == "api" && p[2] == "v1" && p[3] == "ddi" && p[4] == "tenants" && p[6] == "ipam" && p[7] == "spaces" && p[9] == "allocations"
}

type enterpriseResponse struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (w *enterpriseResponse) Header() http.Header { return w.header }
func (w *enterpriseResponse) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *enterpriseResponse) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.body.Write(b)
}

// EnterpriseStaticProblems adapts existing authentication, CSRF and RBAC
// failures to the new route's error contract without changing legacy routes.
func EnterpriseStaticProblems(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isEnterpriseStatic(r) {
			next.ServeHTTP(w, r)
			return
		}
		capture := &enterpriseResponse{header: make(http.Header)}
		next.ServeHTTP(capture, r)
		for key, values := range capture.header {
			w.Header()[key] = append([]string(nil), values...)
		}
		if capture.status >= 400 && !strings.HasPrefix(capture.header.Get("Content-Type"), "application/problem+json") {
			code := "REQUEST_REJECTED"
			title := "Request rejected"
			if capture.status == http.StatusUnauthorized {
				code = "UNAUTHENTICATED"
				title = "Authentication required"
			}
			if capture.status == http.StatusForbidden {
				code = "FORBIDDEN"
				title = "Request forbidden"
			}
			w.Header().Del("Content-Length")
			enterpriseProblem(w, r, capture.status, code, title)
			return
		}
		if capture.status == 0 {
			capture.status = http.StatusOK
		}
		w.WriteHeader(capture.status)
		_, _ = w.Write(capture.body.Bytes())
	})
}

// EnterpriseStaticBodyLimit runs before the existing audit middleware. That
// middleware buffers only the first MiB, so check the complete request here
// before it can truncate a chunked body and hide trailing content.
func EnterpriseStaticBodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !isEnterpriseStatic(r) {
			next.ServeHTTP(w, r)
			return
		}
		data, err := io.ReadAll(io.LimitReader(r.Body, enterpriseBodyLimit+1))
		if err != nil {
			enterpriseProblem(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body")
			return
		}
		if len(data) > enterpriseBodyLimit {
			enterpriseProblem(w, r, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "Request body exceeds limit")
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(data))
		next.ServeHTTP(w, r)
	})
}

func enterpriseProblem(w http.ResponseWriter, r *http.Request, status int, code, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Type      string `json:"type"`
		Title     string `json:"title"`
		Status    int    `json:"status"`
		Code      string `json:"code"`
		RequestID string `json:"requestId"`
	}{"about:blank", detail, status, code, chimw.GetReqID(r.Context())})
}

// UUID input is accepted in standard hyphenated form regardless of hex case.
// Before enabling writes, canonicalize every UUID before constructing paths,
// scopes and digests, so equivalent spellings cannot bypass idempotency.
func canonicalUUID(s string) bool {
	id, e := uuid.Parse(s)
	return len(s) == 36 && e == nil && id != uuid.Nil && id.String() == strings.ToLower(s)
}

// EnterpriseStaticAllocation validates the allocation request and always fails
// closed: identity-to-tenant authorization and writer authority do
// not exist yet. No input can make this handler execute a business write.
func EnterpriseStaticAllocation(w http.ResponseWriter, r *http.Request) {
	bad := func() {
		enterpriseProblem(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Invalid allocation request")
	}
	tenant, space := chi.URLParam(r, "tenantId"), chi.URLParam(r, "spaceId")
	keys := r.Header.Values("Idempotency-Key")
	if !canonicalUUID(tenant) || !canonicalUUID(space) || len(keys) != 1 || !canonicalUUID(keys[0]) || r.URL.RawQuery != "" {
		bad()
		return
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, enterpriseBodyLimit+1))
	if err != nil {
		bad()
		return
	}
	if len(data) > enterpriseBodyLimit {
		enterpriseProblem(w, r, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "Request body exceeds limit")
		return
	}
	if !utf8.Valid(data) || !validJSONSurrogates(data) {
		bad()
		return
	}
	d := json.NewDecoder(bytes.NewReader(data))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		bad()
		return
	}
	fields := map[string]json.RawMessage{}
	for d.More() {
		k, e := d.Token()
		if e != nil {
			bad()
			return
		}
		key, ok := k.(string)
		if !ok {
			bad()
			return
		}
		if _, exists := fields[key]; exists {
			bad()
			return
		}
		switch key {
		case "subnetId", "address", "poolId", "ownerRef", "kind", "holdSeconds":
		default:
			bad()
			return
		}
		var value json.RawMessage
		if e = d.Decode(&value); e != nil {
			bad()
			return
		}
		fields[key] = value
	}
	token, err = d.Token()
	if err != nil || token != json.Delim('}') {
		bad()
		return
	}
	if _, err = d.Token(); !errors.Is(err, io.EOF) {
		bad()
		return
	}
	getString := func(key string) (string, bool) {
		raw, ok := fields[key]
		if !ok {
			return "", false
		}
		var value *string
		if json.Unmarshal(raw, &value) != nil || value == nil {
			return "", false
		}
		return *value, true
	}
	subnet, ok := getString("subnetId")
	if !ok || !canonicalUUID(subnet) {
		bad()
		return
	}
	owner, ok := getString("ownerRef")
	if !ok || utf8.RuneCountInString(owner) < 1 || utf8.RuneCountInString(owner) > 255 {
		bad()
		return
	}
	kind, ok := getString("kind")
	if !ok || (kind != "STATIC" && kind != "HOLD") {
		bad()
		return
	}
	_, hasAddress := fields["address"]
	_, hasPool := fields["poolId"]
	if hasAddress == hasPool {
		bad()
		return
	}
	if hasAddress {
		addr, valid := getString("address")
		if !valid {
			bad()
			return
		}
		if _, e := ipamaddr.ParseAddr(addr); e != nil {
			bad()
			return
		}
	}
	if hasPool {
		pool, valid := getString("poolId")
		if !valid || !canonicalUUID(pool) {
			bad()
			return
		}
	}
	hold, present := fields["holdSeconds"]
	if (kind == "HOLD") != present || (present && !validHoldSeconds(hold)) {
		bad()
		return
	}
	enterpriseProblem(w, r, http.StatusServiceUnavailable, "AUTHORITY_UNAVAILABLE", "Write authority unavailable")
}
