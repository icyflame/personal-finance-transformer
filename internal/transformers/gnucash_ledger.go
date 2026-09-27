package transformers

import (
	"fmt"
	"io"
	"math"
	"os"
	"strings"

	"github.com/icyflame/gnucash-xml-to-ledger-dat/lib/parsers/gnucash"
	"golang.org/x/text/currency"
)

type GnuCashLedgerTransformer struct {
	parser  *gnucash.Parser
	verbose bool
}

func NewGnuCashLedgerTransformer(p *gnucash.Parser, verbose bool) *GnuCashLedgerTransformer {
	return &GnuCashLedgerTransformer{
		parser:  p,
		verbose: verbose,
	}
}

func (t *GnuCashLedgerTransformer) Write(writer io.Writer) error {
	transactions := t.parser.GetTransactions()

	for _, txn := range transactions {
		if len(txn.Splits.Split) == 0 {
			continue
		}

		date := t.formatDate(txn.DatePosted)
		description := t.formatDescription(txn.Description, txn.ID)

		fmt.Fprintf(writer, "%s  %s\n", date, description)

		for _, split := range txn.Splits.Split {
			account := t.parser.GetAccount(split.Account)
			if account == nil {
				continue
			}

			accountName := t.parser.GetAccountName(split.Account)
			commodity := account.Commodity.ID

			// Bug: In a Stock purchase transaction, the "Edit Exchange Rate" dialog is not shown by
			// GnuCash. This seems to be a limitation within the program. If the quantity for a
			// transaction is zero, then it should attempt to use "split:value" instead. If that is zero
			// too, then this split is a no-op and does not need to be included in the resulting Ledger
			// file.  However, for correctness, I *will* include the "0 COMMODITY" split in the output
			// Ledger file.
			quantity := gnucash.ParseFraction(split.Quantity)
			amount := quantity

			value := gnucash.ParseFraction(split.Value)
			if t.verbose {
				fmt.Fprintf(os.Stderr, "%s, %g, %g\n", accountName, quantity, value)
			}
			if quantity == 0 && value != 0 {
				amount = value
				commodity = txn.Currency.ID
			}

			// If the account's commodity is not a real ISO currency, it is a stock symbol. In that
			// case, split:quantity is the number of shares and split:value is the total cost of the
			// split in the transaction's currency. Emit the total cost with the @@ notation so that
			// Ledger can balance transactions in which one stock is traded for another.
			//
			// The total cost must be positive: the sign of the quantity determines whether the split
			// is a buy or a sell. (GnuCash records sells with both quantity and value negative, but
			// hledger rejects a negative total cost since both postings then have the same sign.)
			//
			// See 4.5.2 Buying and Selling Stock in the Ledger manual: https://ledger-cli.org/doc/ledger3.pdf
			if _, err := currency.ParseISO(commodity); err != nil && quantity != 0 && value != 0 {
				fmt.Fprintf(writer, "  %s  \"%s\"  %g @@ \"%s\" %g\n", accountName, commodity, quantity, txn.Currency.ID, math.Abs(value))
				continue
			}

			// Commodity names can have any character (including white space) if they are enclosed in double quotes.
			//
			// See section 4.5.1 Naming Commodities in the Ledger manual: https://ledger-cli.org/doc/ledger3.pdf
			commodity = fmt.Sprintf("\"%s\"", commodity)

			fmt.Fprintf(writer, "  %s  %s  %g\n", accountName, commodity, amount)

			// TODO: Include the second line from the transaction as a comment in the output Ledger file
			// (The second line is visible when the Double line view is enabled in GnuCash)
		}

		fmt.Fprintln(writer)
	}

	return nil
}

func (t *GnuCashLedgerTransformer) formatDate(datePosted string) string {
	// Extract YYYY-MM-DD from "2025-09-01 10:59:00 +0000"
	if len(datePosted) >= 10 {
		return datePosted[:10]
	}
	return datePosted
}

func (t *GnuCashLedgerTransformer) formatDescription(description, txnID string) string {
	description = strings.TrimSpace(description)
	if description == "" {
		return fmt.Sprintf("Empty description (%s)", txnID)
	}
	return description
}

// Sample Ledger transaction:
// 2015/10/12 Exxon
//     Expenses:Auto:Gas         $10.00
//     Liabilities:MasterCard   $-10.00
//
// Sample Stock purchase transaction:
//
// 2004/05/01 Stock purchase
//   Assets:Broker                50 AAPL @ $30.00
//   Expenses:Broker:Commissions  $19.95
//   Assets:Broker                $-1,519.95
//
// The amount can be left out of the last posting:
//
// 2004/05/01 Stock purchase
//   Assets:Broker                50 AAPL @ $30.00
//   Expenses:Broker:Commissions  $19.95
//   Assets:Broker
