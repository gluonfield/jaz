package browserworker

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func browserVersion(ctx context.Context, client *http.Client, port string) (map[string]any, error) {
	var out map[string]any
	err := getJSON(ctx, client, "http://127.0.0.1:"+port+"/json/version", &out)
	return out, err
}

func createTarget(ctx context.Context, client *http.Client, port, targetURL string) (targetInfo, error) {
	endpoint := "http://127.0.0.1:" + port + "/json/new?" + url.QueryEscape(targetURL)
	var out targetInfo
	if err := requestJSON(ctx, client, http.MethodPut, endpoint, &out); err == nil {
		return out, nil
	}
	if err := requestJSON(ctx, client, http.MethodGet, endpoint, &out); err != nil {
		return targetInfo{}, err
	}
	return out, nil
}

func listTargets(ctx context.Context, client *http.Client, port string) ([]targetInfo, error) {
	var out []targetInfo
	err := getJSON(ctx, client, "http://127.0.0.1:"+port+"/json/list", &out)
	return out, err
}

func getJSON(ctx context.Context, client *http.Client, rawURL string, out any) error {
	return requestJSON(ctx, client, http.MethodGet, rawURL, out)
}

func requestJSON(ctx context.Context, client *http.Client, method, rawURL string, out any) error {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return err
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 1024))
		return fmt.Errorf("browser endpoint %s returned %s: %s", rawURL, res.Status, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(res.Body).Decode(out)
}
