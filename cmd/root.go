package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "personal-finance-transformer",
	Short: "Parsers and converters for various personal finance tools",
}

func init() {
	rootCmd.AddCommand(gnucashToLedgerCmd)
	rootCmd.AddCommand(prestiaToLedgerCmd)
	rootCmd.AddCommand(diffLedgerCmd)
	rootCmd.AddCommand(priceFetcherCmd)
}

func Execute() error {
	return rootCmd.Execute()
}
