package handler

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"portfolio-backend/internal/svc"
)

type contractOperation struct {
	Produces   []string                     `json:"produces"`
	Responses  map[string]json.RawMessage   `json:"responses"`
	Security   []map[string]json.RawMessage `json:"security"`
	Parameters []contractParameter          `json:"parameters"`
}

type contractParameter struct {
	Name     string `json:"name"`
	In       string `json:"in"`
	Required bool   `json:"required"`
}

type swaggerContract struct {
	Paths               map[string]map[string]contractOperation `json:"paths"`
	SecurityDefinitions map[string]json.RawMessage              `json:"securityDefinitions"`
}

var apiRouteLine = regexp.MustCompile(`^\s*(get|post|put|patch|delete)\s+(\S+)`)
var routeParameter = regexp.MustCompile(`:([^/]+)`)
var swaggerPathParameter = regexp.MustCompile(`\{([^/{}]+)\}`)

func normalizeContractPath(path string) string {
	return routeParameter.ReplaceAllString(path, `{$1}`)
}

func routeKey(method, path string) string {
	return strings.ToUpper(method) + " " + normalizeContractPath(path)
}

func runtimeContractRoutes() map[string]struct{} {
	routes := make(map[string]struct{})
	for _, route := range registeredRoutes(&svc.ServiceContext{}) {
		if strings.HasPrefix(route.Path, "/swagger") {
			continue
		}
		routes[routeKey(route.Method, route.Path)] = struct{}{}
	}
	return routes
}

func loadAPIContractRoutes(t *testing.T) map[string]struct{} {
	t.Helper()
	file, err := os.Open(filepath.Join("..", "..", "portfolio.api"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	routes := make(map[string]struct{})
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		match := apiRouteLine.FindStringSubmatch(scanner.Text())
		if len(match) == 3 {
			routes[routeKey(match[1], match[2])] = struct{}{}
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return routes
}

func loadSwaggerContract(t *testing.T) swaggerContract {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "swagger.json"))
	if err != nil {
		t.Fatal(err)
	}
	var contract swaggerContract
	if err := json.Unmarshal(data, &contract); err != nil {
		t.Fatal(err)
	}
	return contract
}

func swaggerContractRoutes(contract swaggerContract) map[string]struct{} {
	routes := make(map[string]struct{})
	for path, methods := range contract.Paths {
		for method := range methods {
			routes[routeKey(method, path)] = struct{}{}
		}
	}
	return routes
}

func sortedRouteDifference(left, right map[string]struct{}) []string {
	var difference []string
	for route := range left {
		if _, ok := right[route]; !ok {
			difference = append(difference, route)
		}
	}
	sort.Strings(difference)
	return difference
}

func assertRouteSetsEqual(t *testing.T, label string, expected, actual map[string]struct{}) {
	t.Helper()
	missing := sortedRouteDifference(expected, actual)
	extra := sortedRouteDifference(actual, expected)
	if len(missing) > 0 || len(extra) > 0 {
		t.Fatalf("%s route drift: missing=%v extra=%v", label, missing, extra)
	}
}

func TestRuntimeAPIAndSwaggerRoutesRemainInParity(t *testing.T) {
	runtimeRoutes := runtimeContractRoutes()
	assertRouteSetsEqual(t, "portfolio.api", runtimeRoutes, loadAPIContractRoutes(t))
	assertRouteSetsEqual(t, "swagger.json", runtimeRoutes, swaggerContractRoutes(loadSwaggerContract(t)))
}

func TestSwaggerPathTemplatesDeclareRequiredParameters(t *testing.T) {
	contract := loadSwaggerContract(t)
	for path, operations := range contract.Paths {
		for _, match := range swaggerPathParameter.FindAllStringSubmatch(path, -1) {
			name := match[1]
			for method, operation := range operations {
				found := false
				for _, parameter := range operation.Parameters {
					if parameter.Name == name && parameter.In == "path" && parameter.Required {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("%s %s is missing required path parameter %q", strings.ToUpper(method), path, name)
				}
			}
		}
	}
}

func TestSwaggerCriticalRuntimeContracts(t *testing.T) {
	contract := loadSwaggerContract(t)
	for _, definition := range []string{"AdminSessionCookie", "InternalBearer"} {
		if _, ok := contract.SecurityDefinitions[definition]; !ok {
			t.Fatalf("missing security definition %s", definition)
		}
	}

	for _, item := range []struct{ path, method string }{
		{"/api/ai/chat/stream", "post"},
		{"/api/portfolio/assistant/chat/stream", "post"},
		{"/api/studio/executions/{id}/events", "get"},
	} {
		operation := contract.Paths[item.path][item.method]
		if !containsString(operation.Produces, "text/event-stream") {
			t.Errorf("%s %s must produce text/event-stream", item.method, item.path)
		}
	}

	assertSwaggerResponse(t, contract, "/api/portfolio/assistant/chat/stream", "post", "409")
	assertSwaggerResponse(t, contract, "/api/portfolio/assistant/sessions/{id}/request-human", "post", "409")
	for _, path := range []string{
		"/api/admin/studio/executions/{id}/pause",
		"/api/admin/studio/executions/{id}/approve",
		"/api/ai/contact-summary",
		"/api/jobs/contact-follow-up",
	} {
		assertSwaggerResponse(t, contract, path, "post", "501")
	}

	for path, methods := range contract.Paths {
		for method, operation := range methods {
			requiredScheme := ""
			if strings.HasPrefix(path, "/api/admin/") || path == "/api/ai/contact-summary" {
				requiredScheme = "AdminSessionCookie"
			} else if strings.HasPrefix(path, "/api/jobs/") {
				requiredScheme = "InternalBearer"
			}
			if requiredScheme != "" && !hasSecurityScheme(operation.Security, requiredScheme) {
				t.Errorf("%s %s missing %s security", method, path, requiredScheme)
			}
		}
	}
}

func assertSwaggerResponse(t *testing.T, contract swaggerContract, path, method, status string) {
	t.Helper()
	operation, ok := contract.Paths[path][method]
	if !ok {
		t.Fatalf("missing Swagger operation %s %s", method, path)
	}
	if _, ok := operation.Responses[status]; !ok {
		t.Errorf("%s %s missing %s response", method, path, status)
	}
}

func hasSecurityScheme(security []map[string]json.RawMessage, scheme string) bool {
	for _, option := range security {
		if _, ok := option[scheme]; ok {
			return true
		}
	}
	return false
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func TestRegisteredRoutesDoNotContainDuplicates(t *testing.T) {
	seen := make(map[string]struct{})
	for _, route := range registeredRoutes(&svc.ServiceContext{}) {
		key := fmt.Sprintf("%s %s", route.Method, route.Path)
		if _, exists := seen[key]; exists {
			t.Fatalf("duplicate route %s", key)
		}
		seen[key] = struct{}{}
		if route.Method == "" || route.Path == "" || route.Handler == nil {
			t.Fatalf("invalid registered route: %#v", route)
		}
		if route.Method != http.MethodGet && route.Method != http.MethodPost && route.Method != http.MethodPatch && route.Method != http.MethodDelete {
			t.Fatalf("unexpected method %s", route.Method)
		}
	}
}
