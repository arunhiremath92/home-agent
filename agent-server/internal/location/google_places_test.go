package location

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestGooglePlacesClientResolveAndSearch(t *testing.T) {
	var nearbyTypes []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/geocode":
			if r.URL.Query().Get("latlng") != "40.7000000,-74.0000000" ||
				r.URL.Query().Get("key") != "test-key" {
				t.Errorf("geocode query = %v", r.URL.Query())
			}
			_, _ = w.Write([]byte(`{"status":"OK","results":[{"formatted_address":"Cafe Street","types":["premise","establishment"]}]}`))
		case "/nearby":
			if got := r.Header.Get("X-Goog-Api-Key"); got != "test-key" {
				t.Errorf("X-Goog-Api-Key = %q", got)
			}
			if got := r.Header.Get("X-Goog-FieldMask"); got == "" {
				t.Error("X-Goog-FieldMask is required")
			}
			var body struct {
				IncludedTypes []string `json:"includedTypes"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Errorf("decode nearby request: %v", err)
			}
			nearbyTypes = body.IncludedTypes
			_, _ = w.Write([]byte(`{"places":[{"displayName":{"text":"Market"},"primaryType":"grocery_store","formattedAddress":"1 Main St","location":{"latitude":40.7,"longitude":-74.0},"googleMapsUri":"https://maps.example/market"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := &GooglePlacesClient{
		apiKey:          "test-key",
		httpClient:      server.Client(),
		geocodingURL:    server.URL + "/geocode",
		nearbySearchURL: server.URL + "/nearby",
	}
	resolved, err := client.ResolveLocation(context.Background(), 40.7, -74.0)
	if err != nil {
		t.Fatalf("ResolveLocation() error = %v", err)
	}
	if resolved.Name != "Cafe Street" || resolved.Type != "premise" || len(resolved.AddressTypes) != 2 {
		t.Errorf("ResolveLocation() = %+v", resolved)
	}
	places, err := client.SearchNearby(context.Background(), 40.7, -74.0, []string{"grocery_store"})
	if err != nil {
		t.Fatalf("SearchNearby() error = %v", err)
	}
	if !reflect.DeepEqual(nearbyTypes, []string{"grocery_store"}) {
		t.Errorf("includedTypes = %v", nearbyTypes)
	}
	if len(places) != 1 || places[0].Name != "Market" || places[0].GoogleMapsURL != "https://maps.example/market" {
		t.Errorf("SearchNearby() = %+v", places)
	}
}
