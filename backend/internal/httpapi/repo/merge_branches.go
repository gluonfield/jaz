package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/wins/jaz/backend/internal/gitinfo"
	"github.com/wins/jaz/backend/internal/httpapi"
)

func MergeBranches(w http.ResponseWriter, r *http.Request, cwd string) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if r.Method == http.MethodPost {
		var input struct {
			Source gitinfo.Revision `json:"source"`
			Target string           `json:"target"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, err)
			return
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			httpapi.WriteError(w, http.StatusBadRequest, fmt.Errorf("expected one merge selection"))
			return
		}
		if err := gitinfo.MergeBranchIntoMain(ctx, cwd, input.Source, input.Target); err != nil {
			httpapi.WriteError(w, http.StatusBadRequest, err)
			return
		}
		httpapi.WriteJSON(w, http.StatusOK, gitinfo.Inspect(ctx, cwd))
		return
	}
	branches, err := gitinfo.MergeBranches(ctx, cwd)
	if err != nil {
		httpapi.WriteError(w, http.StatusBadGateway, err)
		return
	}
	httpapi.WriteJSON(w, http.StatusOK, branches)
}
