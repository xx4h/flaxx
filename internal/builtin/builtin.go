package builtin

// Template represents a built-in extra template that can be initialized
// into a user's flux repo.
type Template struct {
	Name        string
	Description string
	Files       map[string]string // filename -> content
}

// All returns all available built-in templates.
func All() []Template {
	return []Template{
		vsoTemplate(),
		ingressTemplate(),
		multusTemplate(),
	}
}

// FindByName returns a built-in template by name, or nil if not found.
func FindByName(name string) *Template {
	for _, t := range All() {
		if t.Name == name {
			return &t
		}
	}
	return nil
}

func vsoTemplate() Template {
	return Template{
		Name:        "vso",
		Description: "Vault Secret Operator auth setup (VaultAuth + ServiceAccount)",
		Files: map[string]string{
			"_meta.yaml": `name: vso
description: Vault Secret Operator auth setup
target: namespaces
variables:
  vault_mount:
    description: Vault auth mount path
    default: "{{.Cluster}}-auth-mount"
  vault_role:
    description: Vault role name
    default: "{{.Cluster}}-{{.App}}-role"
`,
			"serviceaccount.yaml": `---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {{.App}}-vault-sa
`,
			"vso-config.yaml": `---
apiVersion: secrets.hashicorp.com/v1beta1
kind: VaultAuth
metadata:
  name: vso-auth
spec:
  method: kubernetes
  mount: {{.vault_mount}}
  kubernetes:
    role: {{.vault_role}}
    serviceAccount: {{.App}}-vault-sa
    audiences:
      - vault
`,
		},
	}
}

func ingressTemplate() Template {
	return Template{
		Name:        "ingress",
		Description: "Traefik ingress with HTTP redirect and HTTPS termination via cert-manager",
		Files: map[string]string{
			"_meta.yaml": `name: ingress
description: Traefik ingress with HTTP redirect and HTTPS termination
target: namespaces
variables:
  host:
    description: Ingress hostname
    default: "{{.App}}.example.com"
  service_name:
    description: Backend service name
    default: "{{.App}}-http"
  service_port:
    description: Backend service port
    default: "8080"
  redirect_middleware:
    description: Traefik middleware for HTTP to HTTPS redirect
    default: "default-redirect-https@kubernetescrd"
  security_middleware:
    description: Traefik middleware for security headers
    default: "default-security-headers@kubernetescrd"
  cluster_issuer:
    description: cert-manager ClusterIssuer name
    default: "letsencrypt-production"
`,
			"ingress.yaml": `---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: {{.App}}-http
  annotations:
    traefik.ingress.kubernetes.io/router.entrypoints: web
    traefik.ingress.kubernetes.io/router.middlewares: {{.redirect_middleware}}
spec:
  rules:
    - host: {{.host}}
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: {{.service_name}}
                port:
                  number: {{.service_port}}
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: {{.App}}-https
  annotations:
    cert-manager.io/cluster-issuer: {{.cluster_issuer}}
    traefik.ingress.kubernetes.io/router.entrypoints: websecure
    traefik.ingress.kubernetes.io/router.tls: "true"
    traefik.ingress.kubernetes.io/router.middlewares: {{.security_middleware}}
spec:
  rules:
    - host: {{.host}}
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: {{.service_name}}
                port:
                  number: {{.service_port}}
  tls:
  - hosts:
    - {{.host}}
    secretName: {{.host}}-cert-production
`,
		},
	}
}

func multusTemplate() Template {
	return Template{
		Name:        "multus",
		Description: "Multus macvlan NetworkAttachmentDefinition",
		Files: map[string]string{
			"_meta.yaml": `name: multus
description: Multus macvlan NetworkAttachmentDefinition
target: namespaces
variables:
  master_interface:
    description: Host network interface for macvlan
    default: "eth0"
  subnet:
    description: Macvlan subnet
    default: "192.168.1.0/24"
  range_start:
    description: IP range start
    default: "192.168.1.100"
  range_end:
    description: IP range end
    default: "192.168.1.100"
  gateway:
    description: Macvlan gateway
    default: "192.168.1.1"
`,
			"multus-macvlan-conf.yaml": `apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: macvlan-conf
spec:
  config: '{
      "cniVersion": "0.3.0",
      "type": "macvlan",
      "master": "{{.master_interface}}",
      "mode": "bridge",
      "ipam": {
        "type": "host-local",
        "subnet": "{{.subnet}}",
        "rangeStart": "{{.range_start}}",
        "rangeEnd": "{{.range_end}}",
        "routes": [
          {"dst": "0.0.0.0/0"}
        ],
      "gateway": "{{.gateway}}"
      }
    }'
`,
		},
	}
}
