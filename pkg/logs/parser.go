// Package logs provides pod log fetching and one-line root-cause extraction.
package logs

import (
	"context"
	"fmt"
	"io"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

// FetchLogs retrieves up to lines lines of logs from the named container.
// Set previous=true to fetch logs from the last (crashed) container instance.
// Returns the raw log text as a single string, or an error.
func FetchLogs(
	ctx context.Context,
	cs kubernetes.Interface,
	namespace, pod, container string,
	lines int64,
	previous bool,
) (string, error) {
	opts := &corev1.PodLogOptions{
		Container: container,
		TailLines: &lines,
		Previous:  previous,
	}
	req := cs.CoreV1().Pods(namespace).GetLogs(pod, opts)
	rc, err := req.Stream(ctx)
	if err != nil {
		return "", fmt.Errorf("streaming logs for %s/%s[%s]: %w", namespace, pod, container, err)
	}
	defer rc.Close()

	raw, err := io.ReadAll(rc)
	if err != nil {
		return "", fmt.Errorf("reading logs for %s/%s[%s]: %w", namespace, pod, container, err)
	}
	return string(raw), nil
}
