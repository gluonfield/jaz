package oauth

import (
	"io"
	"net/http"
	"net/url"
	"strings"
)

// oauth2.Config does not expose additional parameters for refresh grants.
type resourceTransport struct {
	base     http.RoundTripper
	resource string
}

func (t resourceTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Method != http.MethodPost || req.Body == nil {
		return t.base.RoundTrip(req)
	}
	body, err := io.ReadAll(req.Body)
	req.Body.Close()
	if err != nil {
		return nil, err
	}
	form, err := url.ParseQuery(string(body))
	if err != nil {
		return nil, err
	}
	form.Set("resource", t.resource)
	encoded := form.Encode()
	req = req.Clone(req.Context())
	req.Body = io.NopCloser(strings.NewReader(encoded))
	req.ContentLength = int64(len(encoded))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader(encoded)), nil
	}
	return t.base.RoundTrip(req)
}
