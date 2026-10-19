package drive

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const APIBaseURL = "https://www.googleapis.com"

type APIClient struct {
	HTTP    *http.Client
	BaseURL string
}

type APIError struct {
	StatusCode int
	Message    string
}

func (e APIError) Error() string {
	return fmt.Sprintf("Google Drive API (%d): %s", e.StatusCode, e.Message)
}

func (c APIClient) newRequest(ctx context.Context, method, path string, query url.Values, body io.Reader, contentType string) (*http.Request, error) {
	base := c.BaseURL
	if base == "" {
		base = APIBaseURL
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path+"?"+query.Encode(), body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return req, nil
}

func (c APIClient) do(req *http.Request) (*http.Response, error) {
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp, nil
	}
	defer resp.Body.Close()
	var result struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&result)
	if result.Error.Message == "" {
		result.Error.Message = resp.Status
	}
	return nil, APIError{StatusCode: resp.StatusCode, Message: result.Error.Message}
}

func (c APIClient) json(ctx context.Context, method, path string, query url.Values, input, output any) error {
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := c.newRequest(ctx, method, path, query, body, "application/json")
	if err != nil {
		return err
	}
	resp, err := c.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(output)
}

func filePath(id string) (string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", fmt.Errorf("fileId is required")
	}
	return "/drive/v3/files/" + url.PathEscape(id), nil
}
