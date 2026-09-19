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
	Price    float64   // regularMarketPrice, or the daily close for historical lookups
	Currency string    // e.g. "JPY"
	Date     time.Time // derived from regularMarketTime / bar timestamp (Unix seconds)
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

// chartMeta holds the per-symbol metadata from a chart result.
type chartMeta struct {
	Currency             string  `json:"currency"`
	Symbol               string  `json:"symbol"`
	RegularMarketPrice   float64 `json:"regularMarketPrice"`
	RegularMarketTime    int64   `json:"regularMarketTime"`
	ExchangeTimezoneName string  `json:"exchangeTimezoneName"`
}

// chartResult holds one entry of the chart response's result array.
type chartResult struct {
	Meta       chartMeta `json:"meta"`
	Timestamp  []int64   `json:"timestamp"`
	Indicators struct {
		Quote []struct {
			Close []*float64 `json:"close"`
		} `json:"quote"`
	} `json:"indicators"`
}

// chartResponse is the internal decoding target for the Yahoo Finance JSON.
type chartResponse struct {
	Chart struct {
		Result []chartResult `json:"result"`
		Error  *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

// FetchQuote calls GET /v8/finance/chart/{symbol} and returns the price and
// currency for the given symbol. If date is zero, the latest price is
// returned (interval=1d&range=1d). Otherwise the closing price on that date
// (or the most recent prior trading day) is returned, using
// period1/period2 to fetch daily bars around the date.
func (c *Client) FetchQuote(symbol string, date time.Time) (Quote, error) {
	url := fmt.Sprintf("%s/%s?interval=1d&range=1d", baseURL, symbol)
	if !date.IsZero() {
		// Look back 10 days to cover weekends and long holiday stretches;
		// stop at the end of the requested day.
		period1 := date.AddDate(0, 0, -10).Unix()
		period2 := date.AddDate(0, 0, 1).Unix()
		url = fmt.Sprintf("%s/%s?interval=1d&period1=%d&period2=%d", baseURL, symbol, period1, period2)
	}

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

	result := cr.Chart.Result[0]
	if date.IsZero() {
		return Quote{
			Symbol:   result.Meta.Symbol,
			Price:    result.Meta.RegularMarketPrice,
			Currency: result.Meta.Currency,
			Date:     time.Unix(result.Meta.RegularMarketTime, 0),
		}, nil
	}

	return quoteOnDate(result, date)
}

// quoteOnDate picks the last daily bar whose date in the exchange's local
// timezone is on or before the requested date. meta.regularMarketPrice always
// reflects the latest price, so historical lookups must use the bar arrays.
func quoteOnDate(result chartResult, date time.Time) (Quote, error) {
	loc, err := time.LoadLocation(result.Meta.ExchangeTimezoneName)
	if err != nil {
		loc = time.UTC
	}

	var (
		bestTime  time.Time
		bestClose float64
		found     bool
	)
	if len(result.Indicators.Quote) > 0 {
		closes := result.Indicators.Quote[0].Close
		for i, ts := range result.Timestamp {
			if i >= len(closes) || closes[i] == nil {
				continue
			}
			barTime := time.Unix(ts, 0)
			barDate := barTime.In(loc)
			if barDate.Year() > date.Year() ||
				(barDate.Year() == date.Year() && barDate.YearDay() > date.YearDay()) {
				continue
			}
			if !found || barTime.After(bestTime) {
				bestTime, bestClose, found = barTime, *closes[i], true
			}
		}
	}

	if !found {
		return Quote{}, fmt.Errorf("no trading data for symbol %q on or before %s",
			result.Meta.Symbol, date.Format("2006-01-02"))
	}

	return Quote{
		Symbol:   result.Meta.Symbol,
		Price:    bestClose,
		Currency: result.Meta.Currency,
		Date:     bestTime,
	}, nil
}
