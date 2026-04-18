package importer

import (
	"fmt"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// restConfigT is a local alias so tests can swap in a fake without having to
// import client-go's rest package.
type restConfigT = *rest.Config

// restConfig builds a *rest.Config from the user's kubeconfig. It honors
// (in order): the --kubeconfig flag, the KUBECONFIG env variable, and
// ~/.kube/config. The --context flag overrides the current-context.
func restConfig(kubeconfigPath, contextName string) (*rest.Config, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfigPath != "" {
		loadingRules.ExplicitPath = kubeconfigPath
	}

	overrides := &clientcmd.ConfigOverrides{}
	if contextName != "" {
		overrides.CurrentContext = contextName
	}

	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("building rest config: %w", err)
	}
	return cfg, nil
}
