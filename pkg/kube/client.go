// Package kube provides read-only wrappers around the Kubernetes client-go
// library. All API access for klarity goes through this package — never call
// client-go directly from cmd/ or diagnosis/.
package kube

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// KubeloginVersion holds a parsed semantic version from kubelogin --version.
type KubeloginVersion struct {
	Major int
	Minor int
	Patch int
	Raw   string
}

// String returns the original version string.
func (v KubeloginVersion) String() string { return v.Raw }

// AtLeast returns true if v >= major.minor.patch.
func (v KubeloginVersion) AtLeast(major, minor, patch int) bool {
	if v.Major != major {
		return v.Major > major
	}
	if v.Minor != minor {
		return v.Minor > minor
	}
	return v.Patch >= patch
}

// versionRe matches "v0.1.19" or "0.1.19" anywhere in the output.
var versionRe = regexp.MustCompile(`v?(\d+)\.(\d+)\.(\d+)`)

// DetectKubeloginVersion runs `kubelogin --version` and parses the result.
// Returns the parsed version and true, or a zero value and false if kubelogin
// is not installed, the exec fails, or the output cannot be parsed.
func DetectKubeloginVersion() (KubeloginVersion, bool) {
	out, err := exec.Command("kubelogin", "--version").CombinedOutput()
	if err != nil {
		return KubeloginVersion{}, false
	}
	return parseKubeloginVersion(strings.TrimSpace(string(out)))
}

// parseKubeloginVersion extracts a semver triple from version output.
func parseKubeloginVersion(output string) (KubeloginVersion, bool) {
	m := versionRe.FindStringSubmatch(output)
	if m == nil {
		return KubeloginVersion{}, false
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	patch, _ := strconv.Atoi(m[3])
	return KubeloginVersion{
		Major: major, Minor: minor, Patch: patch,
		Raw: m[0],
	}, true
}

// KubeloginWarning is the advisory message shown when kubelogin >= 0.1.19 is detected.
const KubeloginWarning = `⚠️  kubelogin %s detected. If you're using azurecli auth mode,
   context switching may trigger re-authentication and block scans.
   Pin to v0.1.17 for reliable multi-cluster scanning with Azure CLI auth.
   See: https://github.com/Azure/kubelogin/issues/358`

// CheckKubeloginVersion detects kubelogin and returns a warning string if the
// version is >= 0.1.19. Returns empty string if kubelogin is absent, version
// is acceptable, or detection fails.
func CheckKubeloginVersion() string {
	v, ok := DetectKubeloginVersion()
	if !ok {
		return ""
	}
	if v.AtLeast(0, 1, 19) {
		return fmt.Sprintf(KubeloginWarning, v.Raw)
	}
	return ""
}

// ClientsetBuilder constructs a Kubernetes client for the given context.
// The real implementation calls BuildClientset; tests inject a factory that
// returns a fake.Clientset without needing a live cluster or kubeconfig file.
type ClientsetBuilder func(kubeconfigPath, contextName string) (kubernetes.Interface, error)

// DefaultKubeconfigPath returns $KUBECONFIG if set, otherwise
// ~/.kube/config (the kubectl default).
func DefaultKubeconfigPath() string {
	if v := os.Getenv("KUBECONFIG"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".kube", "config")
}

// defaultQPS and defaultBurst are the rate-limit values applied whenever
// explicit values are not provided. These are 10× the client-go defaults
// (QPS=5, Burst=10), sized for parallel namespace scanning.
const (
	defaultQPS   float32 = 50
	defaultBurst int     = 100
)

// BuildClientset creates a *kubernetes.Clientset for the given kubeconfig
// context, using the package defaults (QPS=50, Burst=100).
// Pass an empty kubeconfigPath to use DefaultKubeconfigPath().
//
// This is the production ClientsetBuilder. Tests should use a fake builder
// instead of calling this function.
func BuildClientset(kubeconfigPath, contextName string) (kubernetes.Interface, error) {
	return BuildClientsetWithRateLimit(kubeconfigPath, contextName, 0, 0)
}

// BuildClientsetWithRateLimit creates a *kubernetes.Clientset with configurable
// client-side rate limits. Pass qps=0 or burst=0 to use the defaults (50/100).
// cmd/root.go calls this with values from cfg.Settings.APIQps / APIBurst so
// users can tune the limiter in ~/.klarityconfig.yaml.
func BuildClientsetWithRateLimit(kubeconfigPath, contextName string, qps float32, burst int) (kubernetes.Interface, error) {
	if kubeconfigPath == "" {
		kubeconfigPath = DefaultKubeconfigPath()
	}
	if qps <= 0 {
		qps = defaultQPS
	}
	if burst <= 0 {
		burst = defaultBurst
	}

	loadingRules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}
	overrides := &clientcmd.ConfigOverrides{CurrentContext: contextName}
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)

	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("context %q: building REST config: %w", contextName, err)
	}

	restConfig.QPS = qps
	restConfig.Burst = burst

	cs, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("context %q: creating clientset: %w", contextName, err)
	}
	return cs, nil
}

