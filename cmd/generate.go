package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/xx4h/flaxx/internal/config"
	"github.com/xx4h/flaxx/internal/extras"
	"github.com/xx4h/flaxx/internal/generator"
)

var (
	deployType  string
	namespace   string
	extraNames  []string
	setVars     []string
	gitURL      string
	gitBranch   string
	gitPath     string
	gitSecret   string
	helmURL     string
	helmChart   string
	helmVersion string
	dryRun      bool
)

var generateCmd = &cobra.Command{
	Use:   "generate <cluster> <app>",
	Short: "Generate scaffolding files for a new Flux app",
	Long:  "Generate all necessary Kustomization, namespace, and source files for deploying a new app via FluxCD.",
	Args:  cobra.ExactArgs(2),
	RunE:  runGenerate,
}

func init() {
	generateCmd.Flags().StringVarP(&deployType, "type", "t", "", "deployment type: core, core-helm, ext-git, ext-helm, ext-oci (required)")
	generateCmd.Flags().StringSliceVarP(&extraNames, "extra", "e", nil, "enable extras by name (repeatable)")
	generateCmd.Flags().StringSliceVar(&setVars, "set", nil, "override template variables (key=value, repeatable)")
	generateCmd.Flags().StringVarP(&namespace, "namespace", "n", "", "override namespace (default: app name)")
	generateCmd.Flags().StringVar(&gitURL, "git-url", "", "Git repository URL (required for ext-git)")
	generateCmd.Flags().StringVar(&gitBranch, "git-branch", "main", "Git branch")
	generateCmd.Flags().StringVar(&gitPath, "git-path", "./deploy/production", "path in external Git repo")
	generateCmd.Flags().StringVar(&gitSecret, "git-secret", "git-repo-secret", "secret name for Git auth")
	generateCmd.Flags().StringVar(&helmURL, "helm-url", "", "Helm repository URL (required for helm types)")
	generateCmd.Flags().StringVar(&helmChart, "helm-chart", "", "Helm chart name (default: app name)")
	generateCmd.Flags().StringVar(&helmVersion, "helm-version", "", "Helm chart version")
	generateCmd.Flags().BoolVar(&dryRun, "dry-run", false, "print output without writing files")

	_ = generateCmd.MarkFlagRequired("type")

	_ = generateCmd.RegisterFlagCompletionFunc("type", completeType)
	_ = generateCmd.RegisterFlagCompletionFunc("extra", completeExtra)

	rootCmd.AddCommand(generateCmd)
}

func runGenerate(cmd *cobra.Command, args []string) error {
	cluster := args[0]
	app := args[1]

	dt := generator.DeployType(deployType)
	if err := validateType(dt); err != nil {
		return err
	}
	if err := validateTypeFlags(dt); err != nil {
		return err
	}

	sets, err := parseSets(setVars)
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

	opts := generator.Options{
		App:         app,
		Cluster:     cluster,
		Namespace:   namespace,
		Type:        dt,
		GitURL:      gitURL,
		GitBranch:   gitBranch,
		GitPath:     gitPath,
		GitSecret:   gitSecret,
		HelmURL:     helmURL,
		HelmChart:   helmChart,
		HelmVersion: helmVersion,
		Extras:      extraNames,
		Sets:        sets,
		DryRun:      dryRun,
	}

	result, err := generator.Run(cfg, opts, cwd)
	if err != nil {
		return err
	}

	if !dryRun {
		fmt.Println("Created files:")
		for _, f := range result.Files {
			fmt.Printf("  %s\n", f)
		}
	}

	return nil
}

func validateType(dt generator.DeployType) error {
	switch dt {
	case generator.TypeCore, generator.TypeCoreHelm, generator.TypeExtGit, generator.TypeExtHelm, generator.TypeExtOCI:
		return nil
	default:
		return fmt.Errorf("invalid type %q: must be one of core, core-helm, ext-git, ext-helm, ext-oci", dt)
	}
}

func validateTypeFlags(dt generator.DeployType) error {
	switch dt {
	case generator.TypeExtGit:
		if gitURL == "" {
			return fmt.Errorf("--git-url is required for type ext-git")
		}
	case generator.TypeCoreHelm, generator.TypeExtHelm, generator.TypeExtOCI:
		if helmURL == "" {
			return fmt.Errorf("--helm-url is required for type %s", dt)
		}
	}
	if dt == generator.TypeExtOCI && !strings.HasPrefix(helmURL, "oci://") {
		return fmt.Errorf("--helm-url must start with oci:// for type ext-oci")
	}
	return nil
}

func parseSets(sets []string) (map[string]string, error) {
	result := make(map[string]string)
	for _, s := range sets {
		parts := strings.SplitN(s, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid --set value %q: expected key=value", s)
		}
		result[parts[0]] = parts[1]
	}
	return result, nil
}

func completeType(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
	return []string{
		"core\tKustomization pointing to local namespace resources",
		"core-helm\tLocal namespace + HelmRepository/HelmRelease",
		"ext-git\tDual Kustomization with external GitRepository",
		"ext-helm\tLocal namespace + external HelmRepository",
		"ext-oci\tLocal namespace + OCI HelmRepository",
	}, cobra.ShellCompDirectiveNoFileComp
}

func completeExtra(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
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
