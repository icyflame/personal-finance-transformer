package frankfurter

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const baseURL = "https://api.frankfurter.dev/v2/rates"

// Rate mirrors one record from the Frankfurter v2 /rates response array.
type Rate struct {
	Date  string  `json:"date"`
	Base  string  `json:"base"`
	Quote string  `json:"quote"`
	Rate  float64 `json:"rate"`
}

// Client wraps an http.Client pointed at the Frankfurter v2 API.
type Client struct {
	httpClient *http.Client
}

// New returns a Client with a 10-second request timeout.
func New() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// FetchRate calls GET /v2/rates?base=<base>&quotes=<quote> and returns the
// single Rate record for that currency pair. If date is non-zero, the rate
// for that date (or the closest prior business day) is returned instead of
// the latest rate.
func (c *Client) FetchRate(base, quote string, date time.Time) (Rate, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return Rate{}, fmt.Errorf("failed to parse base URL: %w", err)
	}

	q := u.Query()
	q.Set("base", base)
	q.Set("quotes", quote)
	if !date.IsZero() {
		q.Set("date", date.Format("2006-01-02"))
	}
	u.RawQuery = q.Encode()

	rates, err := c.fetch(u.String())
	if err != nil {
		return Rate{}, err
	}
	if len(rates) == 0 {
		return Rate{}, fmt.Errorf("no rate returned for %s/%s", base, quote)
	}
	return rates[0], nil
}

// fetch performs the HTTP GET and decodes the []Rate JSON response.
func (c *Client) fetch(rawURL string) ([]Rate, error) {
	resp, err := c.httpClient.Get(rawURL)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d from Frankfurter API: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var rates []Rate
	if err := json.NewDecoder(resp.Body).Decode(&rates); err != nil {
		return nil, fmt.Errorf("failed to decode API response: %w", err)
	}

	return rates, nil
}
