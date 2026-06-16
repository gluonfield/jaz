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
		httpapi.WriteJSON(w, http.StatusOK, map[string]any{"days": days})
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
