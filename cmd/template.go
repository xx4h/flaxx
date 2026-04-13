package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/xx4h/flaxx/internal/builtin"
	"github.com/xx4h/flaxx/internal/config"
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

func init() {
	templateCmd.AddCommand(templateListCmd)
	templateCmd.AddCommand(templateInitCmd)
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
