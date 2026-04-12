package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/xx4h/flaxx/internal/config"
	"github.com/xx4h/flaxx/internal/extras"
	"github.com/xx4h/flaxx/internal/generator"
)

var (
	addExtraNames []string
	addSetVars    []string
	addNamespace  string
	addDryRun     bool
)

var addCmd = &cobra.Command{
	Use:   "add <cluster> <app>",
	Short: "Add extras to an existing app",
	Long:  "Render extra templates into an existing app's directories and update kustomization.yaml.",
	Args:  cobra.ExactArgs(2),
	RunE:  runAdd,
}

func init() {
	addCmd.Flags().StringSliceVarP(&addExtraNames, "extra", "e", nil, "extras to add (repeatable, required)")
	addCmd.Flags().StringSliceVar(&addSetVars, "set", nil, "override template variables (key=value, repeatable)")
	addCmd.Flags().StringVarP(&addNamespace, "namespace", "n", "", "override namespace (default: app name)")
	addCmd.Flags().BoolVar(&addDryRun, "dry-run", false, "print output without writing files")

	_ = addCmd.MarkFlagRequired("extra")

	_ = addCmd.RegisterFlagCompletionFunc("extra", completeAddExtra)

	rootCmd.AddCommand(addCmd)
}

func runAdd(cmd *cobra.Command, args []string) error {
	cluster := args[0]
	app := args[1]

	sets, err := parseSets(addSetVars)
	if err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	var cfg config.Config
	if cfgFile != "" {
		cfg, err = config.Load(cfgFile)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
	} else {
		cfg, _, err = config.LoadFromDir(cwd)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
	}

	opts := generator.AddOptions{
		App:       app,
		Cluster:   cluster,
		Namespace: addNamespace,
		Extras:    addExtraNames,
		Sets:      sets,
		DryRun:    addDryRun,
	}

	result, err := generator.RunAdd(cfg, opts, cwd)
	if err != nil {
		return err
	}

	if !addDryRun {
		fmt.Println("Files:")
		for _, f := range result.Files {
			fmt.Printf("  %s\n", f)
		}
	}

	return nil
}

func completeAddExtra(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	cfg, _, err := config.LoadFromDir(cwd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	discovered, err := extras.Discover(cfg.TemplatesDir)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	var names []string
	for _, e := range discovered {
		desc := e.Meta.Description
		if desc != "" {
			names = append(names, e.Meta.Name+"\t"+desc)
		} else {
			names = append(names, e.Meta.Name)
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}
