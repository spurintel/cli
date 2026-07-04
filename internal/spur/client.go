package spur

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/spurintel/cli/internal/buildinfo"
)

// BaseURL is the root of the Spur Context API.
const BaseURL = "https://api.spur.us"

const maxResponseBodyBytes int64 = 1 << 20

func userAgent() string {
	return fmt.Sprintf("spur/%s (+https://spur.us)", buildinfo.Version)
}

// Client is a stateless-ish wrapper around net/http. Its only mutable state is
// the embedded *http.Client; each API method is safe for concurrent use.
type Client struct {
	Token string
	HTTP  *http.Client
	// Base is the root of the Context API (api.spur.us).
	Base string
	// FeedsBase is the root of the Data Feeds API (feeds.spur.us). It is a
	// distinct host from Base, overridable independently via config.
	FeedsBase string
	// ExportsBase is the root of the Exports API (exports.spur.us). It is a
	// distinct host from Base and FeedsBase, overridable independently via
	// config, and serves the dynamically filtered feed subsets.
	ExportsBase string
}

// NewClient constructs a Client with a sensible default timeout.
func NewClient(token string) *Client {
	return &Client{
		Token:       token,
		HTTP:        &http.Client{Timeout: 15 * time.Second},
		Base:        BaseURL,
		FeedsBase:   FeedsBaseURL,
		ExportsBase: ExportsBaseURL,
	}
}

// IPContextOptions carries optional query arguments for GetIPContext.
// Zero values mean "do not set the parameter".
type IPContextOptions struct {
	// DT, if non-empty, is sent as the dt=YYYYmmdd query arg. Requires an
	// Enterprise subscription with historical access.
	DT string
}

// GetIPContext calls GET /v2/context/{ip} and returns the parsed body plus the
// response-header metadata (quota remaining, result date).
func (c *Client) GetIPContext(ctx context.Context, ip string, opts IPContextOptions) (IPContext, Meta, error) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return IPContext{}, Meta{}, fmt.Errorf("ip is required")
	}

	endpoint := c.Base + "/v2/context/" + url.PathEscape(ip)
	q := url.Values{}
	if opts.DT != "" {
		q.Set("dt", opts.DT)
	}
	if enc := q.Encode(); enc != "" {
		endpoint += "?" + enc
	}

	var out IPContext
	meta, err := c.doJSON(ctx, endpoint, &out)
	if err != nil {
		return IPContext{}, Meta{}, err
	}
	return out, meta, nil
}

// GetTagMetadata calls GET /v2/metadata/tags/{tag}.
func (c *Client) GetTagMetadata(ctx context.Context, tag string) (TagMetadata, error) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return TagMetadata{}, fmt.Errorf("tag is required")
	}
	endpoint := c.Base + "/v2/metadata/tags/" + url.PathEscape(tag)

	var out TagMetadata
	if _, err := c.doJSON(ctx, endpoint, &out); err != nil {
		return TagMetadata{}, err
	}
	return out, nil
}

// GetStatus calls GET /status and returns the token status.
func (c *Client) GetStatus(ctx context.Context) (Status, error) {
	endpoint := c.Base + "/status"
	var out Status
	if _, err := c.doJSON(ctx, endpoint, &out); err != nil {
		return Status{}, err
	}
	return out, nil
}

// doJSON issues a GET with the Token header, decodes the JSON body into dst,
// and extracts the Spur-specific response headers into a Meta value.
//
// dst must be a non-nil pointer.
func (c *Client) doJSON(ctx context.Context, endpoint string, dst any) (Meta, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Meta{}, err
	}
	req.Header.Set("Token", c.Token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent())

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Meta{}, err
	}
	defer resp.Body.Close()

	body, err := readBody(resp.Body, maxResponseBodyBytes)
	if err != nil {
		return Meta{}, err
	}

	meta := parseMeta(resp.Header)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return meta, newAPIError(resp.StatusCode)
	}

	if dst != nil {
		if err := json.Unmarshal(body, dst); err != nil {
			return meta, fmt.Errorf("decode response: %w", err)
		}
	}
	return meta, nil
}

func readBody(r io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("read response body: %w", ErrResponseTooLarge)
	}
	return body, nil
}

// parseMeta extracts x-balance-remaining and x-result-dt from a response. Both
// headers are optional; missing or malformed values yield zero fields.
func parseMeta(h http.Header) Meta {
	m := Meta{}
	if v := h.Get("x-balance-remaining"); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			m.QueriesRemaining = n
		}
	}
	if v := h.Get("x-result-dt"); v != "" {
		m.ResultDate = strings.TrimSpace(v)
	}
	return m
}
