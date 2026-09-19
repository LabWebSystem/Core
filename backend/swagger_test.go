package backend

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSwaggerEndpointsExposeOpenAPIContract(t *testing.T) {
	handler := (&Server{}).Handler()

	t.Run("OpenAPI JSON", func(t *testing.T) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))

		if response.Code != http.StatusOK {
			t.Fatalf("OpenAPI JSON status=%d body=%s", response.Code, response.Body.String())
		}
		if contentType := response.Header().Get("Content-Type"); contentType != "application/json" {
			t.Fatalf("OpenAPI JSON content type=%q", contentType)
		}
		if !json.Valid(response.Body.Bytes()) || !strings.Contains(response.Body.String(), `"openapi":"3.1.0"`) {
			t.Fatalf("OpenAPI JSONが不正です: %s", response.Body.String())
		}
	})

	t.Run("Swagger UI redirect", func(t *testing.T) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/swagger", nil))

		if response.Code != http.StatusPermanentRedirect || response.Header().Get("Location") != "/swagger/" {
			t.Fatalf("Swagger UI redirect status=%d location=%q", response.Code, response.Header().Get("Location"))
		}
	})

	t.Run("Swagger UI", func(t *testing.T) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/swagger/", nil))

		if response.Code != http.StatusOK {
			t.Fatalf("Swagger UI status=%d body=%s", response.Code, response.Body.String())
		}
		if contentType := response.Header().Get("Content-Type"); contentType != "text/html; charset=utf-8" {
			t.Fatalf("Swagger UI content type=%q", contentType)
		}
		for _, expected := range []string{"swagger-ui-dist@5.11.10/swagger-ui.css", "swagger-ui-dist@5.11.10/swagger-ui-bundle.js", `url: "/openapi.json"`} {
			if !strings.Contains(response.Body.String(), expected) {
				t.Fatalf("Swagger UIに%sがありません", expected)
			}
		}
	})
}
