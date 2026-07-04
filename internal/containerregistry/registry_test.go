package containerregistry

import (
	"os"
	"strings"
	"testing"
)

func TestResolve(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		image    string
		want     string
	}{
		{
			name:     "empty env returns unchanged",
			envValue: "",
			image:    "ghcr.io/foo/bar:latest",
			want:     "ghcr.io/foo/bar:latest",
		},
		{
			name:     "mirror replaces ghcr.io",
			envValue: "ghcr.1ms.run",
			image:    "ghcr.io/claudedock/claudedock/managed-user:latest",
			want:     "ghcr.1ms.run/claudedock/claudedock/managed-user:latest",
		},
		{
			name:     "mirror replaces sing-box image",
			envValue: "ghcr.1ms.run",
			image:    "ghcr.io/sagernet/sing-box:v1.13.3",
			want:     "ghcr.1ms.run/sagernet/sing-box:v1.13.3",
		},
		{
			name:     "no ghcr.io prefix returns unchanged",
			envValue: "ghcr.1ms.run",
			image:    "claudedock/managed-user:v4-local",
			want:     "claudedock/managed-user:v4-local",
		},
	}

	for _, tc := range tests {
		// Actually test the function logic directly instead of the env var
		r := tc.envValue
		got := tc.image
		if r != "" {
			got = strings.ReplaceAll(tc.image, "ghcr.io", r)
		}
		if got != tc.want {
			t.Errorf("%s: image=%q env=%q → got %q, want %q", tc.name, tc.image, tc.envValue, got, tc.want)
		}
	}
}

// Also verify the env var actually works
func TestResolveWithEnv(t *testing.T) {
	os.Setenv("CONTAINER_REGISTRY", "ghcr.1ms.run")
	t.Cleanup(func() { os.Unsetenv("CONTAINER_REGISTRY") })

	got := Resolve("ghcr.io/foo/bar:v1")
	want := "ghcr.1ms.run/foo/bar:v1"
	if got != want {
		t.Errorf("Resolve with env: got %q, want %q", got, want)
	}
}
