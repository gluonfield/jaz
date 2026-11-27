package places

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/wins/jaz/backend/internal/mcpsession"
	"github.com/wins/jaz/backend/internal/sessionevents"
)

const (
	ShowMCPToolName = "show_map"
	maxPlaces       = 25
)

type Presenter interface {
	PresentMCP(sessionID string, presentation *sessionevents.PresentationEvent) (*mcp.CallToolResult, any, error)
}

type MCPTools struct {
	Presenter Presenter
	Geocoder  *Photon
}

type ShowInput struct {
	Title  string       `json:"title" jsonschema:"Short map title, for example Coffee near King's Cross."`
	Places []PlaceInput `json:"places" jsonschema:"The places to pin, in the order the answer discusses them. At most 25."`
}

type PlaceInput struct {
	Name      string   `json:"name" jsonschema:"The place's own name."`
	Address   string   `json:"address,omitempty" jsonschema:"Full street address with city and country. Jaz locates places from it."`
	Latitude  *float64 `json:"latitude,omitempty" jsonschema:"Latitude in decimal degrees, only when taken from a source. Pair with longitude."`
	Longitude *float64 `json:"longitude,omitempty" jsonschema:"Longitude in decimal degrees, only when taken from a source. Pair with latitude."`
	Category  string   `json:"category,omitempty" jsonschema:"What kind of place it is, for example Café or Museum."`
	Note      string   `json:"note,omitempty" jsonschema:"One short line on why it is on the map."`
	Rating    float64  `json:"rating,omitempty" jsonschema:"Rating out of 5, from a source."`
	Reviews   int      `json:"reviews,omitempty" jsonschema:"Number of reviews behind the rating."`
	Price     string   `json:"price,omitempty" jsonschema:"Price level as shown locally, for example ££ or $$."`
	URL       string   `json:"url,omitempty" jsonschema:"The place's own website."`
}

func (t *MCPTools) AddTo(server *mcp.Server) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        ShowMCPToolName,
		Title:       "Show places on a map",
		Description: "Shows places on an interactive map in the user's chat, with a card per place and buttons for directions and Google Maps. Use it whenever an answer recommends, compares or locates real places: restaurants, venues, sights, offices, stops on a trip. Give each place its full street address; Jaz finds the coordinates. Do not draw maps with visualise_show_widget.",
	}, t.Show)
}

func (t *MCPTools) Show(ctx context.Context, req *mcp.CallToolRequest, input ShowInput) (*mcp.CallToolResult, any, error) {
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return nil, nil, errors.New("title is required")
	}
	if len(input.Places) == 0 || len(input.Places) > maxPlaces {
		return nil, nil, fmt.Errorf("places must list 1 to %d places", maxPlaces)
	}
	shown := &sessionevents.PlaceMap{Title: title}
	var located, missing []string
	for i, in := range input.Places {
		place, err := clean(in)
		if err != nil {
			return nil, nil, fmt.Errorf("places[%d]: %w", i, err)
		}
		if in.Latitude == nil {
			match, ok, err := t.locate(ctx, place)
			if err != nil {
				return nil, nil, fmt.Errorf("locate %s: %w", place.Name, err)
			}
			if !ok {
				missing = append(missing, place.Name)
				continue
			}
			place.Latitude, place.Longitude = match.Latitude, match.Longitude
			located = append(located, fmt.Sprintf("%s at %.5f, %.5f (%s)", place.Name, match.Latitude, match.Longitude, match.Label))
		}
		shown.Places = append(shown.Places, place)
	}
	if len(shown.Places) == 0 {
		return nil, nil, fmt.Errorf("could not locate %s; give full street addresses or coordinates from a source", strings.Join(missing, ", "))
	}
	result, _, err := t.Presenter.PresentMCP(mcpsession.SessionID(req), &sessionevents.PresentationEvent{Kind: sessionevents.PresentationMap, Map: shown})
	if err != nil {
		return nil, nil, err
	}
	if len(located) > 0 {
		result.Content = append(result.Content, &mcp.TextContent{Text: "Located from addresses: " + strings.Join(located, "; ") + ". If one is wrong, call show_map again with a fuller address."})
	}
	if len(missing) > 0 {
		result.Content = append(result.Content, &mcp.TextContent{Text: "Not found, left off the map: " + strings.Join(missing, ", ") + ". Give a full street address or coordinates from a source to add them."})
	}
	return result, nil, nil
}

// locate tries the place's name with its address, then the address alone,
// since a street address resolves where a venue's name is unknown to OSM.
func (t *MCPTools) locate(ctx context.Context, place sessionevents.Place) (Match, bool, error) {
	queries := []string{place.Name}
	if place.Address != "" {
		queries = []string{place.Name + ", " + place.Address, place.Address}
	}
	for _, query := range queries {
		match, ok, err := t.Geocoder.Geocode(ctx, query)
		if err != nil || ok {
			return match, ok, err
		}
	}
	return Match{}, false, nil
}

func clean(in PlaceInput) (sessionevents.Place, error) {
	place := sessionevents.Place{
		Name:     strings.TrimSpace(in.Name),
		Address:  strings.TrimSpace(in.Address),
		Category: strings.TrimSpace(in.Category),
		Note:     strings.TrimSpace(in.Note),
		Rating:   in.Rating,
		Reviews:  in.Reviews,
		Price:    strings.TrimSpace(in.Price),
		URL:      strings.TrimSpace(in.URL),
	}
	if place.Name == "" {
		return place, errors.New("name is required")
	}
	if (in.Latitude == nil) != (in.Longitude == nil) {
		return place, errors.New("latitude and longitude go together")
	}
	if in.Latitude != nil {
		if *in.Latitude < -90 || *in.Latitude > 90 || *in.Longitude < -180 || *in.Longitude > 180 {
			return place, errors.New("coordinates are out of range")
		}
		place.Latitude, place.Longitude = *in.Latitude, *in.Longitude
	}
	if place.Rating < 0 || place.Rating > 5 || place.Reviews < 0 {
		return place, errors.New("rating must be 0 to 5 and reviews not negative")
	}
	if place.URL != "" {
		if u, err := url.Parse(place.URL); err != nil || u.Scheme != "https" && u.Scheme != "http" || u.Host == "" {
			return place, errors.New("url must be an http or https address")
		}
	}
	return place, nil
}
