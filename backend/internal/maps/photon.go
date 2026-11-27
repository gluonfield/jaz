package maps

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// photonEndpoint is komoot's public Photon geocoder over OpenStreetMap data,
// which needs no key.
const photonEndpoint = "https://photon.komoot.io/api/"

const browserUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

type Photon struct {
	Endpoint string
	Client   *http.Client
}

type Match struct {
	Latitude  float64
	Longitude float64
	Label     string
}

func NewPhoton() *Photon {
	return &Photon{Endpoint: photonEndpoint, Client: &http.Client{Timeout: 10 * time.Second}}
}

// Geocode returns the best match for a free-text query, or false when Photon
// knows no such place.
func (p *Photon) Geocode(ctx context.Context, query string) (Match, bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.Endpoint+"?"+url.Values{"q": {query}, "limit": {"1"}}.Encode(), nil)
	if err != nil {
		return Match{}, false, err
	}
	req.Header.Set("User-Agent", browserUserAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := p.Client.Do(req)
	if err != nil {
		return Match{}, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Match{}, false, fmt.Errorf("geocoder returned %s", resp.Status)
	}
	var body struct {
		Features []struct {
			Geometry struct {
				Coordinates []float64 `json:"coordinates"`
			} `json:"geometry"`
			Properties map[string]any `json:"properties"`
		} `json:"features"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Match{}, false, fmt.Errorf("decode geocoder response: %w", err)
	}
	if len(body.Features) == 0 || len(body.Features[0].Geometry.Coordinates) != 2 {
		return Match{}, false, nil
	}
	feature := body.Features[0]
	return Match{
		Latitude:  feature.Geometry.Coordinates[1],
		Longitude: feature.Geometry.Coordinates[0],
		Label:     label(feature.Properties),
	}, true, nil
}

func label(properties map[string]any) string {
	text := func(key string) string {
		value, _ := properties[key].(string)
		return value
	}
	parts := []string{}
	for _, part := range []string{text("name"), strings.TrimSpace(text("housenumber") + " " + text("street")), text("city"), text("country")} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, ", ")
}
