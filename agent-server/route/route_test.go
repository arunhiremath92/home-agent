package route

import (
	"strings"
	"testing"
)

func TestHomeCoordinatesFromEnvironment(t *testing.T) {
	tests := []struct {
		name      string
		latitude  string
		longitude string
		wantLat   float64
		wantLng   float64
		wantError string
	}{
		{name: "valid coordinates", latitude: "40.7580", longitude: "-73.9855", wantLat: 40.758, wantLng: -73.9855},
		{name: "missing latitude", longitude: "-73.9855", wantError: "HOME_LATITUDE and HOME_LONGITUDE must both be set"},
		{name: "missing longitude", latitude: "40.7580", wantError: "HOME_LATITUDE and HOME_LONGITUDE must both be set"},
		{name: "invalid latitude", latitude: "north", longitude: "-73.9855", wantError: "HOME_LATITUDE must be a number"},
		{name: "NaN latitude", latitude: "NaN", longitude: "-73.9855", wantError: "HOME_LATITUDE must be a number"},
		{name: "out of range latitude", latitude: "91", longitude: "-73.9855", wantError: "HOME_LATITUDE must be a number"},
		{name: "invalid longitude", latitude: "40.7580", longitude: "west", wantError: "HOME_LONGITUDE must be a number"},
		{name: "out of range longitude", latitude: "40.7580", longitude: "-181", wantError: "HOME_LONGITUDE must be a number"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("HOME_LATITUDE", test.latitude)
			t.Setenv("HOME_LONGITUDE", test.longitude)
			latitude, longitude, err := homeCoordinatesFromEnvironment()
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Fatalf("homeCoordinatesFromEnvironment() error = %v, want substring %q", err, test.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("homeCoordinatesFromEnvironment() error = %v", err)
			}
			if latitude != test.wantLat || longitude != test.wantLng {
				t.Errorf("homeCoordinatesFromEnvironment() = (%v, %v), want (%v, %v)", latitude, longitude, test.wantLat, test.wantLng)
			}
		})
	}
}

func TestNewRouteCalculatorRequiresAPIKey(t *testing.T) {
	t.Setenv("GOOGLE_MAPS_API_KEY", "")

	calculator, err := NewRouteCalculator()
	if err == nil {
		t.Fatal("NewRouteCalculator() error = nil, want missing-key error")
	}
	if calculator != nil {
		t.Fatalf("NewRouteCalculator() calculator = %#v, want nil", calculator)
	}
	if !strings.Contains(err.Error(), "GOOGLE_MAPS_API_KEY is not set") {
		t.Errorf("NewRouteCalculator() error = %q, want missing API key", err)
	}
}
