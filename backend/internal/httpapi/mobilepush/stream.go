package mobilepush

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Stream sends the alerts decided while the client listens as server-sent
// events, with a comment heartbeat that keeps idle proxies from closing it.
func (h Handler) Stream(w http.ResponseWriter, r *http.Request) {
	alerts := h.Service.Subscribe(r.Context())
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	controller := http.NewResponseController(w)
	if controller.Flush() != nil {
		return
	}
	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()
	for {
		var err error
		select {
		case alert, ok := <-alerts:
			if !ok {
				return
			}
			data, _ := json.Marshal(alert)
			_, err = fmt.Fprintf(w, "data: %s\n\n", data)
		case <-heartbeat.C:
			_, err = fmt.Fprint(w, ": heartbeat\n\n")
		}
		if err != nil || controller.Flush() != nil {
			return
		}
	}
}
