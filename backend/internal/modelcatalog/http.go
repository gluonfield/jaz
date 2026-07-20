package modelcatalog

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const modelCatalogRequestTimeout = 10 * time.Second

func fetchJSON(ctx context.Context, method, endpoint, apiKey string, input, target any) error {
	ctx, cancel := context.WithTimeout(ctx, modelCatalogRequestTimeout)
	defer cancel()
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("model provider request failed: %d", res.StatusCode)
	}
	return json.NewDecoder(res.Body).Decode(target)
}
