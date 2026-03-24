package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ai-gateway/config"
	"ai-gateway/types"
)

func TestCheckUpstreamModels_OK(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(types.ModelsResponse{
			Object: "list",
			Data: []types.Model{
				{ID: "m1", Object: "model"},
			},
		})
	}))
	defer ts.Close()

	provs := []config.Provider{
		{Name: "p1", APIKey: "k", BaseURL: ts.URL},
	}
	results := CheckUpstreamModels(context.Background(), provs)
	if len(results) != 1 {
		t.Fatalf("len=%d", len(results))
	}
	if !results[0].OK || results[0].ModelCount != 1 || results[0].Provider != "p1" {
		t.Fatalf("%+v", results[0])
	}
}

func TestCheckUpstreamModels_HTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	defer ts.Close()

	provs := []config.Provider{
		{Name: "p1", APIKey: "k", BaseURL: ts.URL},
	}
	results := CheckUpstreamModels(context.Background(), provs)
	if results[0].OK || results[0].HTTPStatus != http.StatusForbidden {
		t.Fatalf("%+v", results[0])
	}
}
