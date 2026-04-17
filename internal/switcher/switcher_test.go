package switcher

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const baseDeployment = `---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
  namespace: myapp
spec:
  replicas: 3
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: myapp
  template:
    metadata:
      labels:
        app.kubernetes.io/name: myapp
    spec:
      containers:
      - name: myapp
        image: nginx:latest
`

const baseStatefulSet = `---
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: myapp
  namespace: myapp
spec:
  replicas: 2
  serviceName: myapp-headless
  updateStrategy:
    type: RollingUpdate
  selector:
    matchLabels:
      app.kubernetes.io/name: myapp
  template:
    metadata:
      labels:
        app.kubernetes.io/name: myapp
    spec:
      containers:
      - name: myapp
        image: nginx:latest
  volumeClaimTemplates:
  - metadata:
      name: data
    spec:
      accessModes: ["ReadWriteOnce"]
      resources:
        requests:
          storage: 1Gi
`

const baseDaemonSet = `---
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: myapp
  namespace: myapp
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: myapp
  template:
    metadata:
      labels:
        app.kubernetes.io/name: myapp
    spec:
      containers:
      - name: myapp
        image: nginx:latest
`

func writeWorkload(t *testing.T, dir, filename, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestSwitchDeploymentToStatefulSet(t *testing.T) {
	dir := t.TempDir()
	writeWorkload(t, dir, "myapp-deployment.yaml", baseDeployment)

	result, err := Switch(dir, Options{App: "myapp", TargetKind: "statefulset"})
	if err != nil {
		t.Fatalf("Switch: %v", err)
	}

	if result.FromKind != "Deployment" || result.ToKind != "StatefulSet" {
		t.Errorf("unexpected kinds: %+v", result)
	}
	if result.RenamedFrom != "myapp-deployment.yaml" || result.RenamedTo != "myapp-statefulset.yaml" {
		t.Errorf("expected rename, got %+v", result)
	}
	if _, err := os.Stat(filepath.Join(dir, "myapp-deployment.yaml")); !os.IsNotExist(err) {
		t.Error("old file should be removed")
	}
	content := readFile(t, filepath.Join(dir, "myapp-statefulset.yaml"))
	for _, want := range []string{"kind: StatefulSet", "serviceName: myapp", "volumeClaimTemplates", "updateStrategy:", "rollingUpdate:"} {
		if !strings.Contains(content, want) {
			t.Errorf("missing %q in:\n%s", want, content)
		}
	}
	if strings.Contains(content, "strategy:\n") {
		t.Errorf("old 'strategy:' key should have been renamed:\n%s", content)
	}
}

func TestSwitchStatefulSetToDeployment(t *testing.T) {
	dir := t.TempDir()
	writeWorkload(t, dir, "myapp-statefulset.yaml", baseStatefulSet)

	_, err := Switch(dir, Options{App: "myapp", TargetKind: "deployment"})
	if err != nil {
		t.Fatalf("Switch: %v", err)
	}
	content := readFile(t, filepath.Join(dir, "myapp-deployment.yaml"))
	if !strings.Contains(content, "kind: Deployment") {
		t.Errorf("expected Deployment kind, got:\n%s", content)
	}
	if strings.Contains(content, "serviceName:") {
		t.Error("serviceName should be removed")
	}
	if strings.Contains(content, "volumeClaimTemplates") {
		t.Error("volumeClaimTemplates should be removed")
	}
	if !strings.Contains(content, "strategy:") {
		t.Errorf("updateStrategy should have been renamed to strategy:\n%s", content)
	}
}

func TestSwitchDeploymentToDaemonSet(t *testing.T) {
	dir := t.TempDir()
	writeWorkload(t, dir, "myapp-deployment.yaml", baseDeployment)

	_, err := Switch(dir, Options{App: "myapp", TargetKind: "daemonset"})
	if err != nil {
		t.Fatalf("Switch: %v", err)
	}
	content := readFile(t, filepath.Join(dir, "myapp-daemonset.yaml"))
	if !strings.Contains(content, "kind: DaemonSet") {
		t.Error("expected DaemonSet")
	}
	if strings.Contains(content, "replicas:") {
		t.Error("replicas should be removed")
	}
}

func TestSwitchDaemonSetToStatefulSet(t *testing.T) {
	dir := t.TempDir()
	writeWorkload(t, dir, "myapp-daemonset.yaml", baseDaemonSet)

	_, err := Switch(dir, Options{App: "myapp", TargetKind: "statefulset"})
	if err != nil {
		t.Fatalf("Switch: %v", err)
	}
	content := readFile(t, filepath.Join(dir, "myapp-statefulset.yaml"))
	if !strings.Contains(content, "replicas: 1") {
		t.Error("replicas should default to 1")
	}
	if !strings.Contains(content, "serviceName: myapp") {
		t.Error("serviceName should default to app name")
	}
	if !strings.Contains(content, "volumeClaimTemplates") {
		t.Error("volumeClaimTemplates should be inserted")
	}
}

func TestSwitchDaemonSetToDeployment(t *testing.T) {
	dir := t.TempDir()
	writeWorkload(t, dir, "myapp-daemonset.yaml", baseDaemonSet)

	_, err := Switch(dir, Options{App: "myapp", TargetKind: "deployment"})
	if err != nil {
		t.Fatalf("Switch: %v", err)
	}
	content := readFile(t, filepath.Join(dir, "myapp-deployment.yaml"))
	if !strings.Contains(content, "replicas: 1") {
		t.Error("replicas should default to 1")
	}
}

func TestSwitchIdentityNoop(t *testing.T) {
	dir := t.TempDir()
	writeWorkload(t, dir, "myapp-deployment.yaml", baseDeployment)

	result, err := Switch(dir, Options{App: "myapp", TargetKind: "deployment"})
	if err != nil {
		t.Fatalf("Switch: %v", err)
	}
	if len(result.UpdatedFiles) != 0 {
		t.Errorf("identity switch should not write, got %+v", result.UpdatedFiles)
	}
	if len(result.Notices) == 0 {
		t.Error("expected a notice for no-op")
	}
}

func TestSwitchMissingWorkload(t *testing.T) {
	dir := t.TempDir()
	writeWorkload(t, dir, "other.yaml", "kind: ConfigMap\nmetadata:\n  name: foo\n")

	_, err := Switch(dir, Options{App: "myapp", TargetKind: "deployment"})
	if err == nil || !strings.Contains(err.Error(), "no Deployment/StatefulSet/DaemonSet") {
		t.Errorf("expected missing-workload error, got %v", err)
	}
}

func TestSwitchMultipleWorkloadsError(t *testing.T) {
	dir := t.TempDir()
	writeWorkload(t, dir, "a-deployment.yaml", baseDeployment)
	writeWorkload(t, dir, "b-daemonset.yaml", baseDaemonSet)

	_, err := Switch(dir, Options{App: "myapp", TargetKind: "statefulset"})
	if err == nil || !strings.Contains(err.Error(), "multiple workloads") {
		t.Errorf("expected multiple-workloads error, got %v", err)
	}
}

func TestSwitchServiceNameOverride(t *testing.T) {
	dir := t.TempDir()
	writeWorkload(t, dir, "myapp-deployment.yaml", baseDeployment)

	_, err := Switch(dir, Options{App: "myapp", TargetKind: "statefulset", ServiceName: "custom-svc"})
	if err != nil {
		t.Fatalf("Switch: %v", err)
	}
	content := readFile(t, filepath.Join(dir, "myapp-statefulset.yaml"))
	if !strings.Contains(content, "serviceName: custom-svc") {
		t.Errorf("expected overridden serviceName, got:\n%s", content)
	}
}

func TestSwitchDryRunDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	writeWorkload(t, dir, "myapp-deployment.yaml", baseDeployment)

	_, err := Switch(dir, Options{App: "myapp", TargetKind: "statefulset", DryRun: true})
	if err != nil {
		t.Fatalf("Switch: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "myapp-statefulset.yaml")); err == nil {
		t.Error("dry-run should not create new file")
	}
	content := readFile(t, filepath.Join(dir, "myapp-deployment.yaml"))
	if !strings.Contains(content, "kind: Deployment") {
		t.Error("dry-run should leave original file intact")
	}
}

func TestSwitchUpdatesKustomization(t *testing.T) {
	dir := t.TempDir()
	writeWorkload(t, dir, "myapp-deployment.yaml", baseDeployment)
	ks := `apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

resources:
- namespace.yaml
- myapp-deployment.yaml
`
	writeWorkload(t, dir, "kustomization.yaml", ks)

	_, err := Switch(dir, Options{App: "myapp", TargetKind: "statefulset"})
	if err != nil {
		t.Fatalf("Switch: %v", err)
	}
	got := readFile(t, filepath.Join(dir, "kustomization.yaml"))
	if !strings.Contains(got, "- myapp-statefulset.yaml") {
		t.Errorf("kustomization not updated:\n%s", got)
	}
	if strings.Contains(got, "- myapp-deployment.yaml") {
		t.Errorf("kustomization still references old filename:\n%s", got)
	}
}

func TestSwitchLeavesNonConventionalFilename(t *testing.T) {
	dir := t.TempDir()
	writeWorkload(t, dir, "workload.yaml", baseDeployment)

	result, err := Switch(dir, Options{App: "myapp", TargetKind: "statefulset"})
	if err != nil {
		t.Fatalf("Switch: %v", err)
	}
	if result.RenamedTo != "" {
		t.Errorf("should not rename non-conventional file, got %+v", result)
	}
	content := readFile(t, filepath.Join(dir, "workload.yaml"))
	if !strings.Contains(content, "kind: StatefulSet") {
		t.Error("file should have been mutated in place")
	}
}
