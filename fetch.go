package sailor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sailorhq/sailor-go/pkg/opts"
)

// httpResponse is the part of a response the consumer uses.
type httpResponse struct {
	status int
	header http.Header
	body   []byte
}

// do performs one request. The x-token header is sent only when useToken is
// true and a token is configured (CONS-HTTP-1, CONS-FB-2). The timeout
// applies to the whole exchange (CONS-CFG-2).
func (c *Consumer) do(parent context.Context, method, target string, body []byte, headers map[string]string, useToken bool, timeout time.Duration) (*httpResponse, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if useToken && c.conn.token != "" {
		req.Header.Set("x-token", c.conn.token)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return &httpResponse{status: resp.StatusCode, header: resp.Header, body: data}, nil
}

// versionOf returns the server's version for a response: ETag verbatim, else
// x-resource-version (CONS-STATE-4). It is "" when the server gives neither.
func versionOf(h http.Header) string {
	if etag := h.Get("ETag"); etag != "" {
		return etag
	}
	return h.Get("x-resource-version")
}

func (c *Consumer) resourceURL(name string) string {
	return fmt.Sprintf("%s/api/v1/resource/%s/%s/%s",
		c.conn.addr, url.PathEscape(c.conn.namespace), url.PathEscape(c.conn.app), url.PathEscape(name))
}

// fetchResource performs GET for a resource. A non-empty ifNoneMatch sends a
// conditional request (CONS-HTTP-2). The caller interprets the status.
func (c *Consumer) fetchResource(parent context.Context, r *resource, ifNoneMatch string) (*httpResponse, error) {
	headers := map[string]string{}
	if ifNoneMatch != "" {
		headers["If-None-Match"] = ifNoneMatch
	}
	resp, err := c.do(parent, http.MethodGet, c.resourceURL(r.name), nil, headers, true, c.requestTimeout)
	if err == nil {
		c.checkEncryptedHeader(r, resp.header)
	}
	return resp, err
}

// checkEncryptedHeader logs, once per resource, a server answer that disagrees
// with the declared Encrypted flag. The declaration always wins (CONS-CRYPT-9).
func (c *Consumer) checkEncryptedHeader(r *resource, h http.Header) {
	raw := h.Get("x-resource-encrypted")
	if raw == "" || (raw == "true") == r.encrypted {
		return
	}
	r.mu.Lock()
	already := r.mismatchLogged
	r.mismatchLogged = true
	r.mu.Unlock()
	if !already {
		c.emit(opts.LevelWarn, "resource.encryption_mismatch",
			"resource", r.name, "declared", r.encrypted, "server", raw)
	}
}

// fallbackURL implements CONS-FB-1.
func fallbackURL(base, app, name string) string {
	return strings.TrimRight(base, "/") + "/" + app + "-" + name + ".sailor.fall"
}

// fetchFallback retrieves the fallback copy of a resource (CONS-FB-2 to CONS-FB-4).
func (c *Consumer) fetchFallback(parent context.Context, r *resource) ([]byte, string, error) {
	base := c.getenv("SAILOR_FALLBACK_BASE_URL")
	if base == "" {
		return nil, "", errf(CodeFallbackFailed, "SAILOR_FALLBACK_BASE_URL is not set")
	}
	target := fallbackURL(base, c.conn.app, r.name)
	resp, err := c.do(parent, http.MethodGet, target, nil, nil, false, c.requestTimeout)
	if err != nil {
		return nil, target, wrapf(CodeFallbackFailed, err, "fetching fallback")
	}
	if resp.status != http.StatusOK {
		return nil, target, errf(CodeFallbackFailed, "fallback answered HTTP %d", resp.status)
	}
	return resp.body, target, nil
}
