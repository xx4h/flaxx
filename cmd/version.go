package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "dev"
	date    = "1970-01-01"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, _ []string) {
		const format = "%-10s %s\n"
		fmt.Fprintf(cmd.OutOrStdout(), format, "Version:", version)
		fmt.Fprintf(cmd.OutOrStdout(), format, "Commit:", commit)
		fmt.Fprintf(cmd.OutOrStdout(), format, "Date:", date)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
