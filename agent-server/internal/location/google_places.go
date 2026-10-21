package location

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultGeocodingEndpoint = "https://maps.googleapis.com/maps/api/geocode/json"
	defaultNearbyEndpoint    = "https://places.googleapis.com/v1/places:searchNearby"
)

type GooglePlacesClient struct {
	apiKey          string
	httpClient      *http.Client
	geocodingURL    string
	nearbySearchURL string
}

func NewGooglePlacesClient() (*GooglePlacesClient, error) {
	apiKey := strings.TrimSpace(os.Getenv("GOOGLE_MAPS_API_KEY"))
	if apiKey == "" {
		return nil, fmt.Errorf("GOOGLE_MAPS_API_KEY is not set")
	}
	return &GooglePlacesClient{
		apiKey:          apiKey,
		httpClient:      &http.Client{Timeout: 10 * time.Second},
		geocodingURL:    defaultGeocodingEndpoint,
		nearbySearchURL: defaultNearbyEndpoint,
	}, nil
}

func (c *GooglePlacesClient) ResolveLocation(ctx context.Context, latitude, longitude float64) (Place, error) {
	query := url.Values{}
	query.Set("latlng", strconv.FormatFloat(latitude, 'f', 7, 64)+","+strconv.FormatFloat(longitude, 'f', 7, 64))
	query.Set("key", c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.geocodingURL+"?"+query.Encode(), nil)
	if err != nil {
		return Place{}, fmt.Errorf("create reverse-geocoding request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Place{}, fmt.Errorf("send reverse-geocoding request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return Place{}, fmt.Errorf("reverse geocoding returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var result struct {
		Status  string `json:"status"`
		Error   string `json:"error_message"`
		Results []struct {
			Address string   `json:"formatted_address"`
			Types   []string `json:"types"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return Place{}, fmt.Errorf("decode reverse-geocoding response: %w", err)
	}
	if result.Status != "OK" || len(result.Results) == 0 {
		return Place{}, fmt.Errorf("reverse geocoding status %q: %s", result.Status, result.Error)
	}
	first := result.Results[0]
	placeType := ""
	if len(first.Types) > 0 {
		placeType = first.Types[0]
	}
	return Place{
		Name:         first.Address,
		Type:         placeType,
		Latitude:     latitude,
		Longitude:    longitude,
		AddressTypes: first.Types,
	}, nil
}

func (c *GooglePlacesClient) SearchNearby(ctx context.Context, latitude, longitude float64, includedTypes []string) ([]Place, error) {
	if len(includedTypes) == 0 {
		return []Place{}, nil
	}
	body, err := json.Marshal(map[string]any{
		"includedTypes":  includedTypes,
		"maxResultCount": 10,
		"locationRestriction": map[string]any{
			"circle": map[string]any{
				"center": map[string]float64{"latitude": latitude, "longitude": longitude},
				"radius": 5000,
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("encode nearby search request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.nearbySearchURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create nearby search request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goog-Api-Key", c.apiKey)
	req.Header.Set("X-Goog-FieldMask", "places.displayName,places.primaryType,places.formattedAddress,places.location,places.googleMapsUri")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send nearby search request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("nearby search returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	var result struct {
		Places []struct {
			DisplayName struct {
				Text string `json:"text"`
			} `json:"displayName"`
			PrimaryType string `json:"primaryType"`
			Address     string `json:"formattedAddress"`
			Location    struct {
				Latitude  float64 `json:"latitude"`
				Longitude float64 `json:"longitude"`
			} `json:"location"`
			MapURL string `json:"googleMapsUri"`
		} `json:"places"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode nearby search response: %w", err)
	}
	places := make([]Place, 0, len(result.Places))
	for _, item := range result.Places {
		places = append(places, Place{
			Name:          item.DisplayName.Text,
			Type:          item.PrimaryType,
			Address:       item.Address,
			Latitude:      item.Location.Latitude,
			Longitude:     item.Location.Longitude,
			GoogleMapsURL: item.MapURL,
		})
	}
	return places, nil
}
