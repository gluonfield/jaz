package maps

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/sessionevents"
)

type shownMaps struct {
	events []*sessionevents.PresentationEvent
}

func (s *shownMaps) PresentMCP(_ string, presentation *sessionevents.PresentationEvent) (*mcp.CallToolResult, any, error) {
	s.events = append(s.events, presentation)
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "shown"}}}, nil, nil
}

// photon answers like komoot's Photon: GeoJSON features with [lon, lat].
func photon(t *testing.T, known map[string]string) (*Photon, *[]string) {
	t.Helper()
	var queries []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		queries = append(queries, query)
		if r.URL.Query().Get("limit") != "1" || !strings.HasPrefix(r.Header.Get("User-Agent"), "Mozilla/5.0") {
			t.Errorf("request = %s, user agent %q", r.URL, r.Header.Get("User-Agent"))
		}
		features := "[]"
		if feature, ok := known[query]; ok {
			features = "[" + feature + "]"
		}
		_, _ = w.Write([]byte(`{"type":"FeatureCollection","features":` + features + `}`))
	}))
	t.Cleanup(server.Close)
	return &Photon{Endpoint: server.URL + "/api/", Client: server.Client()}, &queries
}

func TestShowMapLocatesAddressesKeepsSourcedCoordinatesAndReportsMisses(t *testing.T) {
	geocoder, queries := photon(t, map[string]string{
		"1 Granary Square, London N1C 4AA": `{"type":"Feature","geometry":{"type":"Point","coordinates":[-0.1250,51.5357]},"properties":{"name":"Granary Square","city":"London","country":"United Kingdom"}}`,
	})
	shown := &shownMaps{}
	tools := &MCPTools{Presenter: shown, Geocoder: geocoder}
	lat, lng := 51.5313, -0.1233
	result, _, err := tools.Show(context.Background(), &mcp.CallToolRequest{}, ShowInput{
		Title: " Coffee near King's Cross ",
		Places: []PlaceInput{
			{Name: "Caravan", Address: "1 Granary Square, London N1C 4AA", Category: "Café", Rating: 4.4, Reviews: 3100, URL: "https://caravanandco.com"},
			{Name: "Origin", Latitude: &lat, Longitude: &lng},
			{Name: "Nowhere Café", Address: "1 Imaginary Lane"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"Caravan, 1 Granary Square, London N1C 4AA", "1 Granary Square, London N1C 4AA", "Nowhere Café, 1 Imaginary Lane", "1 Imaginary Lane"}
	if strings.Join(*queries, "|") != strings.Join(want, "|") {
		t.Fatalf("geocoder queries = %q, want %q", *queries, want)
	}
	if len(shown.events) != 1 || shown.events[0].Kind != sessionevents.PresentationMap {
		t.Fatalf("presented %+v", shown.events)
	}
	wire, _ := json.Marshal(shown.events[0].Map)
	if string(wire) != `{"title":"Coffee near King's Cross","places":[{"name":"Caravan","latitude":51.5357,"longitude":-0.125,"address":"1 Granary Square, London N1C 4AA","category":"Café","rating":4.4,"reviews":3100,"url":"https://caravanandco.com"},{"name":"Origin","latitude":51.5313,"longitude":-0.1233}]}` {
		t.Fatalf("map = %s", wire)
	}
	var text []string
	for _, content := range result.Content {
		text = append(text, content.(*mcp.TextContent).Text)
	}
	report := strings.Join(text, "\n")
	if !strings.Contains(report, "Caravan at 51.53570, -0.12500 (Granary Square, London, United Kingdom)") || !strings.Contains(report, "Not found, left off the map: Nowhere Café") {
		t.Fatalf("result = %s", report)
	}
}

func TestShowMapRejectsInputItCannotDraw(t *testing.T) {
	geocoder, _ := photon(t, nil)
	tools := &MCPTools{Presenter: &shownMaps{}, Geocoder: geocoder}
	lat := 51.5
	far := 120.0
	for name, input := range map[string]ShowInput{
		"no places":          {Title: "Map"},
		"half a coordinate":  {Title: "Map", Places: []PlaceInput{{Name: "A", Latitude: &lat}}},
		"latitude off earth": {Title: "Map", Places: []PlaceInput{{Name: "A", Latitude: &far, Longitude: &lat}}},
		"script url":         {Title: "Map", Places: []PlaceInput{{Name: "A", Latitude: &lat, Longitude: &lat, URL: "javascript:alert(1)"}}},
		"nothing found":      {Title: "Map", Places: []PlaceInput{{Name: "Nowhere", Address: "1 Imaginary Lane"}}},
	} {
		if _, _, err := tools.Show(context.Background(), &mcp.CallToolRequest{}, input); err == nil {
			t.Fatalf("%s: accepted", name)
		}
	}
}
