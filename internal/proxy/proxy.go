package proxy

import (
	"io"
	"net/http"
	"time"
)

type Proxy struct {
	client *http.Client
}

func New() *Proxy {
	return &Proxy{
		client: &http.Client{
			Timeout: 0, // per-request context controls cancellation; 0 lets streams run
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

// Forward sends the buffered request body to backendURL+path and streams the
// response back to w. Body is passed in (not read from r) so later phases can
// inspect it for caching and rate limiting.
func (p *Proxy) Forward(w http.ResponseWriter, r *http.Request, backendURL, path string, body io.Reader) error {
	req, err := http.NewRequestWithContext(r.Context(), r.Method, backendURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	for k, vals := range resp.Header {
		for _, v := range vals {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	flusher, _ := w.(http.Flusher)
	buf := make([]byte, 4096)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return werr
			}
			if flusher != nil {
				flusher.Flush()
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}
