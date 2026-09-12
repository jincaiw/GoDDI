package api

import (
	"encoding/json"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

// The OpenAPI catalogue is assembled from two tables: `routeDocs` carries the
// path parameters (the names inside braces) and `documentedQueryParams` carries
// the query parameters. Every name in `routeDocs.Params` is emitted as
// `in: path, required: true`, so a query parameter listed there is documented
// as a required path segment -- a generated client would then build a URL like
// `/dns/zones?page=2` into `/dns/zones/page%3D2` and get a 404.
//
// That is exactly the class of error this test refuses to let through: the
// spec is the contract, and a contract that describes the wrong location for a
// parameter is worse than one that omits it, because it is trusted.
func TestEveryDocumentedPathParameterIsInThePathTemplate(t *testing.T) {
	rec := httptest.NewRecorder()
	OpenAPIHandler(rec, httptest.NewRequest("GET", "/api/v1/openapi.json", nil))

	var doc struct {
		Paths map[string]map[string]struct {
			Parameters []struct {
				Name     string `json:"name"`
				In       string `json:"in"`
				Required bool   `json:"required"`
			} `json:"parameters"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("openapi.json is not decodable: %v", err)
	}
	if len(doc.Paths) == 0 {
		t.Fatal("no paths documented")
	}

	braces := regexp.MustCompile(`\{([^}]+)\}`)
	for path, methods := range doc.Paths {
		inTemplate := map[string]bool{}
		for _, name := range braces.FindAllStringSubmatch(path, -1) {
			inTemplate[name[1]] = true
		}
		for method, op := range methods {
			inPathParams := map[string]bool{}
			for _, param := range op.Parameters {
				if param.In != "path" {
					continue
				}
				inPathParams[param.Name] = true
				if !inTemplate[param.Name] {
					t.Errorf("%s %s documents %q as a path parameter, but the path has no {%s}: "+
						"a query parameter belongs in documentedQueryParams",
						method, path, param.Name, param.Name)
				}
				if !param.Required {
					t.Errorf("%s %s path parameter %q must be required", method, path, param.Name)
				}
			}
			for name := range inTemplate {
				if !inPathParams[name] {
					t.Errorf("%s %s has {%s} in its path but does not document it", method, path, name)
				}
			}
		}
	}
}

// Every documented operation must carry an operationId, because that is the
// name a generated client calls. Two routes that collapse to the same id would
// silently overwrite each other in the generated code.
func TestEveryDocumentedOperationHasAUniqueID(t *testing.T) {
	rec := httptest.NewRecorder()
	OpenAPIHandler(rec, httptest.NewRequest("GET", "/api/v1/openapi.json", nil))

	var doc struct {
		Paths map[string]map[string]struct {
			OperationID string `json:"operationId"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for path, methods := range doc.Paths {
		for method, op := range methods {
			where := method + " " + path
			if op.OperationID == "" {
				t.Errorf("%s has no operationId", where)
				continue
			}
			if other, ok := seen[op.OperationID]; ok {
				t.Errorf("operationId %q is used by both %s and %s", op.OperationID, other, where)
			}
			seen[op.OperationID] = where
		}
	}
}

// The two resource types that share a zone id are the reason the revision list
// takes a type as well as an id (`dns_zone` and `dns_records` are both keyed by
// the zone). If that parameter were ever dropped from the contract, a client
// would ask for one resource's history and be handed two interleaved.
func TestTheRevisionListDocumentsBothItsNarrowingParameters(t *testing.T) {
	rec := httptest.NewRecorder()
	OpenAPIHandler(rec, httptest.NewRequest("GET", "/api/v1/openapi.json", nil))

	var doc struct {
		Paths map[string]map[string]struct {
			Parameters []struct {
				Name string `json:"name"`
				In   string `json:"in"`
			} `json:"parameters"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	params := map[string]string{}
	for _, p := range doc.Paths["/config/revisions"]["get"].Parameters {
		params[p.Name] = p.In
	}
	for _, want := range []string{"resource_type", "resource_id"} {
		if params[want] != "query" {
			t.Errorf("/config/revisions %s is %q, want query", want, params[want])
		}
	}
}

// The 360 view is addressed by an (space, ip) pair, not by a row id: the caller
// usually knows the address it is asking about but not whether a row exists for
// it yet, so the row is created on read. Both halves must therefore travel as
// query parameters -- a client that thought `ip` was a path segment would build
// `/ipam/addresses/view/10.0.0.1`, which is a different route entirely.
func TestReservedExtensionsDocumentTheir501Contract(t *testing.T) {
	rec := httptest.NewRecorder()
	OpenAPIHandler(rec, httptest.NewRequest("GET", "/api/v1/openapi.json", nil))

	var doc struct {
		Paths map[string]map[string]struct {
			Responses map[string]struct {
				Description string `json:"description"`
			} `json:"responses"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"GET /sso":                "预留扩展：当前版本不提供 SSO 配置",
		"PUT /sso":                "预留扩展：当前版本不提供 SSO 配置",
		"GET /cluster":            "预留扩展：当前版本不提供多节点集群协调",
		"POST /cluster":           "预留扩展：当前版本不提供多节点集群协调",
		"GET /apps":               "预留扩展：当前版本不提供应用市场运行时",
		"POST /apps/{id}/install": "预留扩展：当前版本不提供应用市场运行时",
		"GET /dhcp/ha":            "预留扩展：当前版本不提供该配置 API",
	}
	for key, description := range want {
		parts := regexp.MustCompile(`^([A-Z]+) (.+)$`).FindStringSubmatch(key)
		if len(parts) != 3 {
			t.Fatalf("invalid test key %q", key)
		}
		op, ok := doc.Paths[parts[2]][strings.ToLower(parts[1])]
		if !ok {
			t.Errorf("OpenAPI is missing reserved route %s", key)
			continue
		}
		if got := op.Responses["501"].Description; got != description {
			t.Errorf("%s 501 description = %q, want %q", key, got, description)
		}
	}
}

func TestThe360ViewAddressesAnAddressByQueryNotByPath(t *testing.T) {
	rec := httptest.NewRecorder()
	OpenAPIHandler(rec, httptest.NewRequest("GET", "/api/v1/openapi.json", nil))

	var doc struct {
		Paths map[string]map[string]struct {
			Parameters []struct {
				Name     string `json:"name"`
				In       string `json:"in"`
				Required bool   `json:"required"`
			} `json:"parameters"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatal(err)
	}
	op, ok := doc.Paths["/ipam/addresses/view"]["get"]
	if !ok {
		t.Fatal("/ipam/addresses/view is not documented")
	}
	params := map[string]struct {
		in       string
		required bool
	}{}
	for _, p := range op.Parameters {
		params[p.Name] = struct {
			in       string
			required bool
		}{p.In, p.Required}
	}
	for _, name := range []string{"space_id", "ip"} {
		got, ok := params[name]
		if !ok {
			t.Errorf("/ipam/addresses/view does not document %q", name)
			continue
		}
		if got.in != "query" {
			t.Errorf("/ipam/addresses/view %s is in %q, want query", name, got.in)
		}
		if !got.required {
			t.Errorf("/ipam/addresses/view %s must be required: without it there is no address", name)
		}
	}
}
