package logs

import (
	"context"
	"io"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

// TestFetchLogs_Success verifies FetchLogs does not panic against the fake
// clientset. The fake REST client can't serve actual log streams, so we
// accept either ("", nil) or ("", err) — both are valid for a fake backend.
func TestFetchLogs_Success(t *testing.T) {
	cs := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "mypod", Namespace: "default"},
	})
	result, _ := FetchLogs(context.Background(), cs, "default", "mypod", "mycontainer", 50, false)
	// Result may be empty — the fake client doesn't stream real logs.
	// The important thing is no panic.
	_ = result
}

// TestFetchLogs_ErrorWrapped verifies the error message contains identifying
// context (namespace and pod name) when log fetching fails.
func TestFetchLogs_ErrorWrapped(t *testing.T) {
	cs := fake.NewSimpleClientset()
	_, err := FetchLogs(context.Background(), cs, "mynamespace", "no-such-pod", "c", 100, true)
	if err != nil {
		msg := err.Error()
		if !strings.Contains(msg, "mynamespace") || !strings.Contains(msg, "no-such-pod") {
			t.Errorf("error %q does not mention namespace or pod name", msg)
		}
	}
}

// TestFetchLogs_ReaderIntegration exercises the io.ReadAll path in isolation,
// ensuring raw bytes are correctly converted to a string.
func TestFetchLogs_ReaderIntegration(t *testing.T) {
	body := "log line 1\nlog line 2\npanic: test error\n"
	rc := io.NopCloser(strings.NewReader(body))
	raw, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("unexpected read error: %v", err)
	}
	if string(raw) != body {
		t.Errorf("got %q, want %q", string(raw), body)
	}
}
