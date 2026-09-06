package yahoo

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const baseURL = "https://query1.finance.yahoo.com/v8/finance/chart"

// Quote holds the relevant fields from a Yahoo Finance chart response.
type Quote struct {
	Symbol   string    // e.g. "2633.T"
	Price    float64   // regularMarketPrice
	Currency string    // e.g. "JPY"
	Date     time.Time // derived from regularMarketTime (Unix seconds)
}

// Client wraps an http.Client pointed at the Yahoo Finance chart API.
type Client struct {
	httpClient *http.Client
}

// New returns a Client with a 10-second request timeout.
func New() *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// chartResponse is the internal decoding target for the Yahoo Finance JSON.
type chartResponse struct {
	Chart struct {
		Result []struct {
			Meta struct {
				Currency           string  `json:"currency"`
				Symbol             string  `json:"symbol"`
				RegularMarketPrice float64 `json:"regularMarketPrice"`
				RegularMarketTime  int64   `json:"regularMarketTime"`
			} `json:"meta"`
		} `json:"result"`
		Error *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

// FetchQuote calls GET /v8/finance/chart/{symbol}?interval=1d&range=1d and
// returns the latest price and currency for the given symbol.
func (c *Client) FetchQuote(symbol string) (Quote, error) {
	url := fmt.Sprintf("%s/%s?interval=1d&range=1d", baseURL, symbol)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return Quote{}, fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; price-fetcher/1.0)")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return Quote{}, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return Quote{}, fmt.Errorf("unexpected status %d from Yahoo Finance API: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var cr chartResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return Quote{}, fmt.Errorf("failed to decode API response: %w", err)
	}

	if cr.Chart.Error != nil {
		return Quote{}, fmt.Errorf("Yahoo Finance API error %s: %s", cr.Chart.Error.Code, cr.Chart.Error.Description)
	}

	if len(cr.Chart.Result) == 0 {
		return Quote{}, fmt.Errorf("no data returned for symbol %q", symbol)
	}

	meta := cr.Chart.Result[0].Meta
	return Quote{
		Symbol:   meta.Symbol,
		Price:    meta.RegularMarketPrice,
		Currency: meta.Currency,
		Date:     time.Unix(meta.RegularMarketTime, 0),
	}, nil
}
