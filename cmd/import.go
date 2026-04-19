package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/xx4h/flaxx/internal/config"
	"github.com/xx4h/flaxx/internal/importer"
)

var (
	importKubeconfig     string
	importContext        string
	importNamespace      string
	importIncludeSecrets bool
	importHelmURL        string
	importNonHelm        bool
	importForce          bool
	importDryRun         bool
)

var importCmd = &cobra.Command{
	Use:   "import <cluster> <app>",
	Short: "Adopt an app already running in a cluster into the flaxx repo",
	Long: `Read an app that is currently running in a Kubernetes cluster and write a
matching flaxx-shaped app folder into the repository.

If the app was installed via Helm (flaxx detects a Helm release secret
matching the app name), the adopted output is a single HelmRelease +
HelmRepository pair — equivalent to what 'flaxx generate --type ext-helm'
would have produced.

Otherwise, every user-facing namespaced resource in the namespace is listed,
sanitized (status, managedFields, resourceVersion, cluster-defaulted networking
fields, etc. are stripped), and written as plain YAML. The adopted app is
then wrapped in a core-type Flux Kustomization — equivalent to
'flaxx generate --type core'.

By default, Secrets are skipped; pass --include-secrets to include them
(base64 data is preserved as-is — encryption integration is a future feature).

Examples:
  # Adopt a Helm-installed app
  flaxx import production grafana --namespace monitoring

  # Adopt raw manifests
  flaxx import staging demo-raw

  # Force raw-manifest output even if a Helm release exists
  flaxx import production legacy-app --non-helm

  # Use a specific kubeconfig/context
  flaxx import homelab traefik --kubeconfig ~/.kube/homelab --context homelab-admin`,
	Args:              cobra.ExactArgs(2),
	RunE:              runImport,
	ValidArgsFunction: completeClusterAndApp,
}

func init() {
	importCmd.Flags().StringVar(&importKubeconfig, "kubeconfig", "", "path to kubeconfig (default: $KUBECONFIG or ~/.kube/config)")
	importCmd.Flags().StringVar(&importContext, "context", "", "kubeconfig context to use (default: current context)")
	importCmd.Flags().StringVarP(&importNamespace, "namespace", "n", "", "override namespace to read from (default: app name)")
	importCmd.Flags().BoolVar(&importIncludeSecrets, "include-secrets", false, "include Secret manifests in the adopted output")
	importCmd.Flags().StringVar(&importHelmURL, "helm-url", "", "Helm repository URL to use when auto-detection cannot resolve it")
	importCmd.Flags().BoolVar(&importNonHelm, "non-helm", false, "skip Helm detection and always emit raw manifests")
	importCmd.Flags().BoolVar(&importForce, "force", false, "overwrite an existing app directory")
	importCmd.Flags().BoolVar(&importDryRun, "dry-run", false, "print what would be written without modifying anything")

	rootCmd.AddCommand(importCmd)
}

func runImport(_ *cobra.Command, args []string) error {
	cluster := args[0]
	app := args[1]

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

	result, err := importer.Run(importer.Options{
		App:            app,
		Cluster:        cluster,
		Namespace:      importNamespace,
		Kubeconfig:     importKubeconfig,
		Context:        importContext,
		IncludeSecrets: importIncludeSecrets,
		ForceNonHelm:   importNonHelm,
		HelmURLFlag:    importHelmURL,
		Force:          importForce,
		DryRun:         importDryRun,
		Cfg:            cfg,
		RepoRoot:       cwd,
	})
	if err != nil {
		return err
	}

	if importDryRun {
		return nil
	}

	switch result.Mode {
	case "helm":
		fmt.Printf("Imported %s as a HelmRelease (chart=%s, repo=%s)\n", app, result.HelmChart, result.HelmURL)
	case "raw":
		fmt.Printf("Imported %s as %d raw manifest(s)\n", app, len(result.Files))
	}

	fmt.Println("Created files:")
	for _, f := range result.Files {
		fmt.Printf("  %s\n", f)
	}

	if len(result.Skipped) > 0 {
		fmt.Fprintf(os.Stderr, "\nSkipped %d resource(s):\n", len(result.Skipped))
		for _, s := range result.Skipped {
			fmt.Fprintf(os.Stderr, "  %s\n", s)
		}
	}

	return nil
}
