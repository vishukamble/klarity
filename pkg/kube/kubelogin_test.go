package kube

import "testing"

func TestParseKubeloginVersion(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		wantOK  bool
		wantVer KubeloginVersion
	}{
		{
			name:    "standard version string",
			output:  "v0.1.17",
			wantOK:  true,
			wantVer: KubeloginVersion{Major: 0, Minor: 1, Patch: 17, Raw: "v0.1.17"},
		},
		{
			name:    "version without v prefix",
			output:  "0.1.19",
			wantOK:  true,
			wantVer: KubeloginVersion{Major: 0, Minor: 1, Patch: 19, Raw: "0.1.19"},
		},
		{
			name:    "version embedded in longer output",
			output:  "kubelogin version v0.1.19\ngit hash: abc123",
			wantOK:  true,
			wantVer: KubeloginVersion{Major: 0, Minor: 1, Patch: 19, Raw: "v0.1.19"},
		},
		{
			name:    "major version",
			output:  "v1.0.0",
			wantOK:  true,
			wantVer: KubeloginVersion{Major: 1, Minor: 0, Patch: 0, Raw: "v1.0.0"},
		},
		{
			name:   "empty output",
			output: "",
			wantOK: false,
		},
		{
			name:   "garbage output",
			output: "command not found",
			wantOK: false,
		},
		{
			name:   "partial version",
			output: "v0.1",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, ok := parseKubeloginVersion(tt.output)
			if ok != tt.wantOK {
				t.Fatalf("ok: want %v, got %v", tt.wantOK, ok)
			}
			if !ok {
				return
			}
			if v.Major != tt.wantVer.Major || v.Minor != tt.wantVer.Minor || v.Patch != tt.wantVer.Patch {
				t.Errorf("version: want %d.%d.%d, got %d.%d.%d",
					tt.wantVer.Major, tt.wantVer.Minor, tt.wantVer.Patch,
					v.Major, v.Minor, v.Patch)
			}
			if v.Raw != tt.wantVer.Raw {
				t.Errorf("raw: want %q, got %q", tt.wantVer.Raw, v.Raw)
			}
		})
	}
}

func TestKubeloginVersionAtLeast(t *testing.T) {
	tests := []struct {
		name                  string
		ver                   KubeloginVersion
		major, minor, patch   int
		want                  bool
	}{
		{"exact match", KubeloginVersion{0, 1, 19, ""}, 0, 1, 19, true},
		{"older patch", KubeloginVersion{0, 1, 17, ""}, 0, 1, 19, false},
		{"newer patch", KubeloginVersion{0, 1, 20, ""}, 0, 1, 19, true},
		{"newer minor", KubeloginVersion{0, 2, 0, ""}, 0, 1, 19, true},
		{"older minor", KubeloginVersion{0, 0, 99, ""}, 0, 1, 19, false},
		{"newer major", KubeloginVersion{1, 0, 0, ""}, 0, 1, 19, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.ver.AtLeast(tt.major, tt.minor, tt.patch)
			if got != tt.want {
				t.Errorf("(%d.%d.%d).AtLeast(%d, %d, %d): want %v, got %v",
					tt.ver.Major, tt.ver.Minor, tt.ver.Patch,
					tt.major, tt.minor, tt.patch, tt.want, got)
			}
		})
	}
}

func TestCheckKubeloginVersion_NoWarningBelowThreshold(t *testing.T) {
	// parseKubeloginVersion for a safe version should not produce a warning.
	v, ok := parseKubeloginVersion("v0.1.17")
	if !ok {
		t.Fatal("expected parse to succeed")
	}
	if v.AtLeast(0, 1, 19) {
		t.Error("v0.1.17 should not be >= 0.1.19")
	}
}

func TestCheckKubeloginVersion_WarningAtThreshold(t *testing.T) {
	v, ok := parseKubeloginVersion("v0.1.19")
	if !ok {
		t.Fatal("expected parse to succeed")
	}
	if !v.AtLeast(0, 1, 19) {
		t.Error("v0.1.19 should be >= 0.1.19")
	}
}
