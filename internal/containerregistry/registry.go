package containerregistry

import (
	"os"
	"strings"
)

// Resolve replaces ghcr.io with CONTAINER_REGISTRY when set.
// When CONTAINER_REGISTRY is empty, the image reference is returned unchanged.
func Resolve(image string) string {
	r := os.Getenv("CONTAINER_REGISTRY")
	if r == "" {
		return image
	}
	return strings.ReplaceAll(image, "ghcr.io", r)
}
