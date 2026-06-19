package usage

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/httpapi"
	usagecore "github.com/wins/jaz/backend/internal/usage"
)

type dailyHandler struct {
	service usagecore.Service
}

type dailyResponse struct {
	Days []dailyUsageDTO `json:"days"`
}

type dailyUsageDTO struct {
	Date         string          `json:"date"`
	Usage        usageTotalsDTO  `json:"usage"`
	Models       []modelUsageDTO `json:"models,omitempty"`
	SessionCount int             `json:"session_count"`
}

type usageTotalsDTO struct {
	InputTokens           int64 `json:"input_tokens,omitempty"`
	CachedInputTokens     int64 `json:"cached_input_tokens,omitempty"`
	CachedWriteTokens     int64 `json:"cached_write_tokens,omitempty"`
	OutputTokens          int64 `json:"output_tokens,omitempty"`
	ReasoningOutputTokens int64 `json:"reasoning_output_tokens,omitempty"`
	InputOutputTokens     int64 `json:"input_output_tokens,omitempty"`
}

func NewDailyHandler(service usagecore.Service) http.Handler {
	return dailyHandler{service: service}
}

func (h dailyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	query, err := parseDailyQuery(r)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadRequest, err)
		return
	}
	days, err := h.service.Daily(query)
	switch {
	case errors.Is(err, usagecore.ErrUnsupported):
		httpapi.WriteError(w, http.StatusNotImplemented, err)
	case errors.Is(err, usagecore.ErrInvalidDays):
		httpapi.WriteError(w, http.StatusBadRequest, err)
	case err != nil:
		httpapi.WriteError(w, http.StatusInternalServerError, err)
	default:
		httpapi.WriteJSON(w, http.StatusOK, dailyResponse{Days: dailyDTOs(days)})
	}
}

func dailyDTOs(days []usagecore.DailyBucket) []dailyUsageDTO {
	out := make([]dailyUsageDTO, len(days))
	for i, day := range days {
		out[i] = dailyUsageDTO{
			Date:         day.Date,
			Usage:        usageDTO(day.Usage),
			Models:       modelDTOs(day.Models),
			SessionCount: day.SessionCount,
		}
	}
	return out
}

func usageDTO(usage usagecore.UsageTotals) usageTotalsDTO {
	return usageTotalsDTO{
		InputTokens:           usage.InputTokens,
		CachedInputTokens:     usage.CachedInputTokens,
		CachedWriteTokens:     usage.CachedWriteTokens,
		OutputTokens:          usage.OutputTokens,
		ReasoningOutputTokens: usage.ReasoningOutputTokens,
		InputOutputTokens:     usage.InputOutputTokens(),
	}
}

func parseDailyQuery(r *http.Request) (usagecore.DailyQuery, error) {
	days, err := parseDays(r.URL.Query().Get("days"))
	if err != nil {
		return usagecore.DailyQuery{}, err
	}
	loc, err := parseLocation(r.URL.Query().Get("timezone"), r.URL.Query().Get("tz_offset_minutes"))
	if err != nil {
		return usagecore.DailyQuery{}, err
	}
	return usagecore.DailyQuery{Days: days, Location: loc}, nil
}

func parseDays(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, nil
	}
	days, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, usagecore.ErrInvalidDays
	}
	if days == 0 {
		return 0, usagecore.ErrInvalidDays
	}
	return days, nil
}

func parseLocation(timezone, offset string) (*time.Location, error) {
	if raw := strings.TrimSpace(timezone); raw != "" {
		loc, err := time.LoadLocation(raw)
		if err != nil {
			return nil, fmt.Errorf("timezone must be an IANA timezone: %w", err)
		}
		return loc, nil
	}
	if raw := strings.TrimSpace(offset); raw != "" {
		minutes, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("tz_offset_minutes must be an integer")
		}
		return time.FixedZone("client", -minutes*60), nil
	}
	return nil, nil
}
