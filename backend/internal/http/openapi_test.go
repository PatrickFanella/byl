package httpapi

import (
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"
)

var paramRe = regexp.MustCompile(`\{[^}]+\}`)

func normalisePath(p string) string {
	p = paramRe.ReplaceAllString(p, "{}")
	p = strings.ReplaceAll(p, "/*", "")
	return strings.TrimSuffix(p, "/")
}

func TestRouterMatchesOpenAPI(t *testing.T) {
	raw, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Paths map[string]map[string]any `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}

	documented := map[string]bool{}
	for path, ops := range doc.Paths {
		for method := range ops {
			switch method {
			case "get", "post", "put", "patch", "delete":
				documented[strings.ToUpper(method)+" "+normalisePath(path)] = true
			}
		}
	}

	served := map[string]bool{}
	router := NewRouter(Deps{Sessions: fakeResolver{}}).(chi.Router)
	err = chi.Walk(router, func(method, route string, _ http.Handler, _ ...func(http.Handler) http.Handler) error {
		if strings.HasPrefix(route, "/media/files") {
			return nil
		}
		served[method+" "+normalisePath(route)] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	var missing, extra []string
	for k := range served {
		if !documented[k] {
			missing = append(missing, k)
		}
	}
	for k := range documented {
		if !served[k] {
			extra = append(extra, k)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 0 || len(extra) > 0 {
		t.Fatalf("OpenAPI drift.\nserved but undocumented:\n  %s\ndocumented but not served:\n  %s",
			strings.Join(missing, "\n  "), strings.Join(extra, "\n  "))
	}
	if len(served) == 0 {
		t.Fatal("router served no routes")
	}
}

func TestOpenAPIRefsResolve(t *testing.T) {
	raw, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Components struct {
			Schemas map[string]any `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	refRe := regexp.MustCompile(`#/components/schemas/([A-Za-z]+)`)
	for _, m := range refRe.FindAllStringSubmatch(string(raw), -1) {
		if _, ok := doc.Components.Schemas[m[1]]; !ok {
			t.Errorf("unresolved $ref %s", m[0])
		}
	}
	if len(doc.Components.Schemas) < 20 {
		t.Fatalf("expected the response and request schemas to be documented, found %d", len(doc.Components.Schemas))
	}
}
