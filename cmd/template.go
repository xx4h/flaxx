package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/xx4h/flaxx/internal/builtin"
	"github.com/xx4h/flaxx/internal/config"
	"github.com/xx4h/flaxx/internal/extractor"
)

var templateCmd = &cobra.Command{
	Use:   "template",
	Short: "Manage extra templates",
	Long:  "List available built-in templates or initialize them into your flux repo.",
}

var templateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available built-in templates",
	Args:  cobra.NoArgs,
	Run:   runTemplateList,
}

var templateInitCmd = &cobra.Command{
	Use:               "init <name> [name...]",
	Short:             "Initialize built-in templates into your flux repo",
	Long:              "Write built-in template files to the configured templates directory.",
	Args:              cobra.MinimumNArgs(1),
	RunE:              runTemplateInit,
	ValidArgsFunction: completeTemplateName,
}

var (
	fromAppIncludeCluster bool
	fromAppInteractive    bool
	fromAppForce          bool
	fromAppDryRun         bool
	fromAppDescription    string
)

var templateFromAppCmd = &cobra.Command{
	Use:   "from-app <cluster> <app> <template-name>",
	Short: "Create a reusable template from an existing app",
	Long: `Read the files of an existing Flux app and write them as a reusable
extra template under .flaxx/templates/<template-name>/, replacing the app,
cluster and namespace with {{.App}}/{{.Cluster}}/{{.Namespace}} and offering
detected helm chart, image tag, ingress host and git values as variables.

By default only the app's namespace-directory files are captured. Pass
--include-cluster to also capture the cluster-directory files (Flux
Kustomization, HelmRelease, GitRepository) — the resulting template will
use the split target layout with cluster/ and namespaces/ subdirectories.`,
	Args:              cobra.ExactArgs(3),
	RunE:              runTemplateFromApp,
	ValidArgsFunction: completeClusterAppAndName,
}

func init() {
	templateCmd.AddCommand(templateListCmd)
	templateCmd.AddCommand(templateInitCmd)

	templateFromAppCmd.Flags().BoolVar(&fromAppIncludeCluster, "include-cluster", false, "also capture cluster-dir files (Flux Kustomization/HelmRelease/GitRepository)")
	templateFromAppCmd.Flags().BoolVarP(&fromAppInteractive, "interactive", "i", false, "confirm each detected variable before writing")
	templateFromAppCmd.Flags().BoolVar(&fromAppForce, "force", false, "overwrite an existing template of the same name")
	templateFromAppCmd.Flags().BoolVar(&fromAppDryRun, "dry-run", false, "print what would be written without modifying anything")
	templateFromAppCmd.Flags().StringVar(&fromAppDescription, "description", "", "description written into _meta.yaml")
	templateCmd.AddCommand(templateFromAppCmd)

	rootCmd.AddCommand(templateCmd)
}

func runTemplateList(_ *cobra.Command, _ []string) {
	templates := builtin.All()
	for _, t := range templates {
		fmt.Printf("  %-12s %s\n", t.Name, t.Description)
	}
}

func runTemplateInit(_ *cobra.Command, args []string) error {
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

	templatesDir := filepath.Join(cwd, cfg.TemplatesDir)

	for _, name := range args {
		tmpl := builtin.FindByName(name)
		if tmpl == nil {
			return fmt.Errorf("unknown template %q (use 'flaxx template list' to see available templates)", name)
		}

		targetDir := filepath.Join(templatesDir, tmpl.Name)
		if _, err := os.Stat(targetDir); err == nil {
			return fmt.Errorf("template %q already exists at %s", name, targetDir)
		}

		if err := os.MkdirAll(targetDir, 0o755); err != nil {
			return fmt.Errorf("creating directory %s: %w", targetDir, err)
		}

		for fileName, content := range tmpl.Files {
			filePath := filepath.Join(targetDir, fileName)
			if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil { //nolint:gosec // template files need to be readable
				return fmt.Errorf("writing %s: %w", filePath, err)
			}
		}

		fmt.Printf("Initialized template %q in %s\n", name, targetDir)
	}

	return nil
}

func completeTemplateName(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	var names []string
	for _, t := range builtin.All() {
		names = append(names, t.Name+"\t"+t.Description)
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

// completeClusterAppAndName dispatches to completeClusterAndApp for the first
// two positional args; the third (template name) has no completion.
func completeClusterAppAndName(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) < 2 {
		return completeClusterAndApp(cmd, args, toComplete)
	}
	return nil, cobra.ShellCompDirectiveNoFileComp
}

func runTemplateFromApp(_ *cobra.Command, args []string) error {
	cluster := args[0]
	app := args[1]
	name := args[2]

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

	opts := extractor.ExtractOptions{
		App:            app,
		Cluster:        cluster,
		TemplateName:   name,
		Description:    fromAppDescription,
		IncludeCluster: fromAppIncludeCluster,
		Interactive:    fromAppInteractive,
		Force:          fromAppForce,
		DryRun:         fromAppDryRun,
		Cfg:            cfg,
		RepoRoot:       cwd,
	}
	if fromAppInteractive {
		opts.Prompter = extractor.NewStdinPrompter(os.Stdin, os.Stdout)
	}

	result, err := extractor.Extract(opts)
	if err != nil {
		return err
	}

	if fromAppDryRun {
		return nil
	}

	rel, relErr := filepath.Rel(cwd, result.TemplateDir)
	if relErr != nil {
		rel = result.TemplateDir
	}
	fmt.Printf("Created template %q at %s\n", name, rel)
	for _, f := range result.Files {
		r, rErr := filepath.Rel(cwd, f)
		if rErr != nil {
			r = f
		}
		fmt.Printf("  %s\n", r)
	}
	if len(result.Variables) > 0 {
		fmt.Printf("Detected %d variable(s)\n", len(result.Variables))
	}
	return nil
}
