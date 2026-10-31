package repo

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/wins/jaz/backend/internal/gitinfo"
)

func MergeBranches(w http.ResponseWriter, r *http.Request, cwd string) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	branches, err := gitinfo.MergeBranches(ctx, cwd)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	_ = json.NewEncoder(w).Encode(branches)
}
