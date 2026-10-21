package route

import (
	"context"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	routing "cloud.google.com/go/maps/routing/apiv2"
	routespb "cloud.google.com/go/maps/routing/apiv2/routingpb"
	"google.golang.org/api/option"
	"google.golang.org/genproto/googleapis/type/latlng"
	"google.golang.org/grpc/metadata"
)

const (
	fieldMask = "routes.distanceMeters,routes.duration"
)

type RouteCalculator struct {
	client *routing.RoutesClient
}

func NewRouteCalculator() (*RouteCalculator, error) {
	apiKey := strings.TrimSpace(os.Getenv("GOOGLE_MAPS_API_KEY"))
	if apiKey == "" {
		return nil, fmt.Errorf("GOOGLE_MAPS_API_KEY is not set")
	}
	client, err := routing.NewRoutesClient(context.Background(), option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create Routes client: %v", err)
	}
	return &RouteCalculator{client: client}, nil
}

func (rc *RouteCalculator) Close() error {
	return rc.client.Close()
}

func (rc *RouteCalculator) CalculateRoutes(destinationLat, destinationLng float64) ([]*routespb.Route, error) {
	originLat, originLng, err := homeCoordinatesFromEnvironment()
	if err != nil {
		return nil, err
	}
	return rc.CalculateRoutesFrom(originLat, originLng, destinationLat, destinationLng)
}

func homeCoordinatesFromEnvironment() (float64, float64, error) {
	latitude := strings.TrimSpace(os.Getenv("HOME_LATITUDE"))
	longitude := strings.TrimSpace(os.Getenv("HOME_LONGITUDE"))
	if latitude == "" || longitude == "" {
		return 0, 0, fmt.Errorf("HOME_LATITUDE and HOME_LONGITUDE must both be set when no home location is configured")
	}

	lat, err := strconv.ParseFloat(latitude, 64)
	if err != nil || math.IsNaN(lat) || lat < -90 || lat > 90 {
		return 0, 0, fmt.Errorf("HOME_LATITUDE must be a number between -90 and 90")
	}
	lng, err := strconv.ParseFloat(longitude, 64)
	if err != nil || math.IsNaN(lng) || lng < -180 || lng > 180 {
		return 0, 0, fmt.Errorf("HOME_LONGITUDE must be a number between -180 and 180")
	}
	return lat, lng, nil
}

func (rc *RouteCalculator) CalculateRoutesFrom(originLat, originLng, destinationLat, destinationLng float64) ([]*routespb.Route, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ctx = metadata.AppendToOutgoingContext(ctx, "X-Goog-Fieldmask", fieldMask)

	// create the origin using a latitude and longitude
	origin := &routespb.Waypoint{
		LocationType: &routespb.Waypoint_Location{
			Location: &routespb.Location{
				LatLng: &latlng.LatLng{
					Latitude:  originLat,
					Longitude: originLng,
				},
			},
		},
	}

	// create the destination using a latitude and longitude
	destination := &routespb.Waypoint{
		LocationType: &routespb.Waypoint_Location{
			Location: &routespb.Location{
				LatLng: &latlng.LatLng{
					Latitude:  destinationLat,
					Longitude: destinationLng,
				},
			},
		},
	}
	req := &routespb.ComputeRoutesRequest{
		Origin:                   origin,
		Destination:              destination,
		TravelMode:               routespb.RouteTravelMode_DRIVE,
		RoutingPreference:        routespb.RoutingPreference_TRAFFIC_AWARE,
		ComputeAlternativeRoutes: true,
		Units:                    routespb.Units_METRIC,
		RouteModifiers: &routespb.RouteModifiers{
			AvoidTolls:    false,
			AvoidHighways: true,
			AvoidFerries:  true,
		},
	}

	// execute rpc
	resp, err := rc.client.ComputeRoutes(ctx, req)

	if err != nil {
		// "rpc error: code = InvalidArgument desc = Request contains an invalid
		// argument" may indicate that your project lacks access to Routes
		return nil, fmt.Errorf("failed to compute routes: %v", err)
	}

	if len(resp.Routes) == 0 {
		return nil, fmt.Errorf("no routes returned")
	}

	return resp.Routes, nil
}
