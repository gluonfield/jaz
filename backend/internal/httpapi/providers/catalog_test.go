package providers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/modelcatalog"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestCatalogHTTPMapsInvalidRequestsAndMissingProviders(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	handler := NewCatalogHandler(modelcatalog.Discovery{Store: store})
	for _, test := range []struct {
		body   string
		status int
	}{
		{`{`, http.StatusBadRequest}, {`{}`, http.StatusBadRequest}, {`{"id":"missing"}`, http.StatusNotFound},
		{`{"base_url":"https://example.com","save":true}`, http.StatusBadRequest},
		{strings.Repeat(" ", 16385) + `{}`, http.StatusBadRequest},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/", strings.NewReader(test.body)))
		if response.Code != test.status {
			t.Fatalf("status=%d want=%d: %s", response.Code, test.status, response.Body.String())
		}
	}
}
