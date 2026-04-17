package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/xx4h/flaxx/internal/config"
	"github.com/xx4h/flaxx/internal/generator"
	"github.com/xx4h/flaxx/internal/switcher"
	"github.com/xx4h/flaxx/internal/templates"
)

var (
	switchKind        string
	switchServiceName string
	switchNamespace   string
	switchDryRun      bool
)

var switchCmd = &cobra.Command{
	Use:   "switch <cluster> <app>",
	Short: "Switch a workload between Deployment, StatefulSet, and DaemonSet",
	Long: `Convert the workload manifest for an app from its current kind (Deployment,
StatefulSet, or DaemonSet) to a different kind. Kind-specific fields are added
or removed as needed (e.g. serviceName + volumeClaimTemplates for StatefulSet,
replicas dropped for DaemonSet).

Examples:
  # Convert a Deployment to a StatefulSet
  flaxx switch k8s myapp --kind statefulset

  # Override the default serviceName
  flaxx switch k8s myapp --kind statefulset --service-name myapp-headless

  # Preview the change
  flaxx switch k8s myapp --kind daemonset --dry-run`,
	Args:              cobra.ExactArgs(2),
	RunE:              runSwitch,
	ValidArgsFunction: completeClusterAndApp,
}

func init() {
	switchCmd.Flags().StringVar(&switchKind, "kind", "", "target workload kind (deployment|statefulset|daemonset)")
	switchCmd.Flags().StringVar(&switchServiceName, "service-name", "", "serviceName for StatefulSet (default: metadata.name)")
	switchCmd.Flags().StringVarP(&switchNamespace, "namespace", "n", "", "override namespace (default: app name)")
	switchCmd.Flags().BoolVar(&switchDryRun, "dry-run", false, "print output without writing files")

	_ = switchCmd.MarkFlagRequired("kind")
	_ = switchCmd.RegisterFlagCompletionFunc("kind", completeWorkloadKind)

	rootCmd.AddCommand(switchCmd)
}

func runSwitch(_ *cobra.Command, args []string) error {
	cluster := args[0]
	app := args[1]

	if templates.NormalizeWorkloadKind(switchKind) == "" {
		return fmt.Errorf("invalid --kind %q: want deployment|statefulset|daemonset", switchKind)
	}

	ns := switchNamespace
	if ns == "" {
		ns = app
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

	genOpts := generator.Options{App: app, Cluster: cluster, Namespace: ns}
	namespacesDir, err := generator.ResolvePath(cfg.Paths.NamespacesDir, genOpts)
	if err != nil {
		return err
	}
	appNamespacesDir := filepath.Join(cwd, namespacesDir, app)

	result, err := switcher.Switch(appNamespacesDir, switcher.Options{
		App:         app,
		Namespace:   ns,
		TargetKind:  switchKind,
		ServiceName: switchServiceName,
		DryRun:      switchDryRun,
	})
	if err != nil {
		return err
	}

	for _, n := range result.Notices {
		fmt.Fprintln(os.Stderr, "notice: "+n)
	}

	if switchDryRun {
		return nil
	}

	if result.FromKind == result.ToKind {
		return nil
	}

	fmt.Printf("Switched %s → %s\n", result.FromKind, result.ToKind)
	if result.RenamedFrom != "" {
		fmt.Printf("  renamed: %s → %s\n", result.RenamedFrom, result.RenamedTo)
	}
	fmt.Println("Updated files:")
	for _, f := range result.UpdatedFiles {
		fmt.Printf("  %s\n", f)
	}
	return nil
}
