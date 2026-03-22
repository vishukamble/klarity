package cmd

import (
	"reflect"
	"testing"
)

func TestParseCommaSeparated(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "empty string",
			input: "",
			want:  nil,
		},
		{
			name:  "single value",
			input: "payments",
			want:  []string{"payments"},
		},
		{
			name:  "multiple values",
			input: "payments,analytics,orders",
			want:  []string{"payments", "analytics", "orders"},
		},
		{
			name:  "spaces around commas",
			input: " payments , analytics , orders ",
			want:  []string{"payments", "analytics", "orders"},
		},
		{
			name:  "trailing comma",
			input: "payments,",
			want:  []string{"payments"},
		},
		{
			name:  "only commas",
			input: ",,",
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCommaSeparated(tt.input)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseCommaSeparated(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestApplyNamespaceFilters(t *testing.T) {
	tests := []struct {
		name       string
		namespaces []string
		nsInclude  []string
		nsExclude  []string
		want       []string
	}{
		{
			name:       "no filters",
			namespaces: []string{"payments", "analytics", "orders"},
			nsInclude:  nil,
			nsExclude:  nil,
			want:       []string{"payments", "analytics", "orders"},
		},
		{
			name:       "include filter (passthrough, handled upstream)",
			namespaces: []string{"payments", "analytics"},
			nsInclude:  []string{"payments", "analytics"},
			nsExclude:  nil,
			want:       []string{"payments", "analytics"},
		},
		{
			name:       "exclude filter",
			namespaces: []string{"payments", "build-ns-1", "analytics", "build-ns-2"},
			nsInclude:  nil,
			nsExclude:  []string{"build-ns-1", "build-ns-2"},
			want:       []string{"payments", "analytics"},
		},
		{
			name:       "include wins over exclude",
			namespaces: []string{"payments", "analytics"},
			nsInclude:  []string{"payments", "analytics"},
			nsExclude:  []string{"analytics"},
			want:       []string{"payments", "analytics"},
		},
		{
			name:       "exclude namespace not in list (no-op)",
			namespaces: []string{"payments", "analytics"},
			nsInclude:  nil,
			nsExclude:  []string{"nonexistent"},
			want:       []string{"payments", "analytics"},
		},
		{
			name:       "exclude all namespaces",
			namespaces: []string{"payments", "analytics"},
			nsInclude:  nil,
			nsExclude:  []string{"payments", "analytics"},
			want:       nil,
		},
		{
			name:       "empty namespace list",
			namespaces: nil,
			nsInclude:  nil,
			nsExclude:  []string{"something"},
			want:       nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyNamespaceFilters(tt.namespaces, tt.nsInclude, tt.nsExclude)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("applyNamespaceFilters(%v, %v, %v) = %v, want %v",
					tt.namespaces, tt.nsInclude, tt.nsExclude, got, tt.want)
			}
		})
	}
}
