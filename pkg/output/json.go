package output

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/vishukamble/klarity/pkg/diagnosis"
)

// jsonFinding is the JSON-serialisable form of a diagnosis.Finding.
// All fields use snake_case keys. No ANSI codes — this file must never import
// lipgloss or call any function from color.go / table.go.
type jsonFinding struct {
	Category   string            `json:"category"`
	Severity   string            `json:"severity"`
	Env        string            `json:"env"`
	Cluster    string            `json:"cluster"`
	Namespace  string            `json:"namespace"`
	Pod        string            `json:"pod,omitempty"`
	Container  string            `json:"container,omitempty"`
	Summary    string            `json:"summary"`
	Detail     map[string]string `json:"detail,omitempty"`
}

// RenderJSON writes findings as a JSON array to w.
// It never calls lipgloss — safe to use when stdout is not a TTY.
func RenderJSON(findings []diagnosis.Finding, w io.Writer) error {
	out := make([]jsonFinding, 0, len(findings))
	for _, f := range findings {
		jf := jsonFinding{
			Category:  string(f.Category),
			Severity:  string(f.Severity),
			Env:       f.EnvName,
			Cluster:   f.ClusterCtx,
			Namespace: f.Namespace,
			Pod:       f.PodName,
			Container: f.ContainerName,
			Summary:   f.OneLiner,
			Detail:    f.DetailFields,
		}
		out = append(out, jf)
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("encoding findings as JSON: %w", err)
	}
	return nil
}
