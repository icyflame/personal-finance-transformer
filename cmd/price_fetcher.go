package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/icyflame/gnucash-xml-to-ledger-dat/lib/frankfurter"
	"github.com/icyflame/gnucash-xml-to-ledger-dat/lib/yahoo"
	"github.com/spf13/cobra"
	"golang.org/x/text/currency"
)

var priceFetcherCurrencyBase string

var priceFetcherCmd = &cobra.Command{
	Use:   "price-fetcher <commodities-file>",
	Short: "Fetch prices for currencies and stocks/ETFs",
	Long: `Read a file containing commodity names (one per line) and fetch the latest prices.

Valid ISO 4217 currency codes are fetched from the Frankfurter API against the
given --base-currency. All other commodities are treated as stock/ETF tickers
and fetched from Yahoo Finance (priced in their native currency).

Output is written to stdout as Ledger price directives:

  P DATE <CURRENCY> <BASE-CURRENCY> <RATE>      (for currencies)
  P DATE "<TICKER>" <PRICE> "<NATIVE-CURRENCY>"  (for stocks/ETFs)

Use "-" as the filename to read from stdin.`,
	Args: cobra.ExactArgs(1),
	RunE: runPriceFetcher,
}

func init() {
	priceFetcherCmd.Flags().StringVar(&priceFetcherCurrencyBase, "base-currency", "", "Base currency for exchange rates (required, e.g. JPY)")
	priceFetcherCmd.MarkFlagRequired("base-currency")
}

func runPriceFetcher(cmd *cobra.Command, args []string) error {
	// Open input.
	var file *os.File
	if args[0] == "-" {
		file = os.Stdin
	} else {
		var err error
		file, err = os.Open(args[0])
		if err != nil {
			return fmt.Errorf("failed to open commodities file: %w", err)
		}
		defer file.Close()
	}

	// Scan lines: valid ISO currencies go to Frankfurter, everything else to Yahoo Finance.
	var currencies []string
	var stocks []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if unit, err := currency.ParseISO(line); err == nil {
			currencies = append(currencies, unit.String())
		} else {
			stocks = append(stocks, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to read commodities file: %w", err)
	}

	if len(currencies) == 0 && len(stocks) == 0 {
		return fmt.Errorf("no commodities found in input file")
	}

	// Fetch currency exchange rates from Frankfurter.
	// One call per currency: base=<currency>&quotes=<base-currency>
	// gives "1 <currency> = X <base-currency>", mapping directly to the directive.
	frankfurterClient := frankfurter.New()
	for _, cur := range currencies {
		rate, err := frankfurterClient.FetchRate(cur, priceFetcherCurrencyBase)
		if err != nil {
			return fmt.Errorf("failed to fetch exchange rate for %s: %w", cur, err)
		}
		fmt.Fprintf(os.Stdout, "P %s %s %s %v\n", rate.Date, rate.Base, rate.Quote, rate.Rate)
	}

	// Fetch stock/ETF prices from Yahoo Finance.
	// Price is in the symbol's native currency; --base-currency is not used here.
	yahooClient := yahoo.New()
	for _, symbol := range stocks {
		quote, err := yahooClient.FetchQuote(symbol)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to fetch quote for %q: %v, skipping\n", symbol, err)
			continue
		}
		fmt.Fprintf(os.Stdout, "P %s \"%s\" %v \"%s\"\n",
			quote.Date.Format("2006-01-02"), quote.Symbol, quote.Price, quote.Currency)
	}

	return nil
}
