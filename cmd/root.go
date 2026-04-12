package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var cfgFile string

var rootCmd = &cobra.Command{
	Use:   "flaxx",
	Short: "Generic Flux app scaffolding tool",
	Long:  "flaxx generates the boilerplate files needed to deploy a new app in a FluxCD GitOps repository.",
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: auto-detect .flaxx.yaml)")
}
