package sshproxy

import (
	"context"
	"crypto/subtle"
	"fmt"
	"os/exec"
	"strings"

	"github.com/zanel1u/cloud-cli-proxy/internal/store/repository"
)

type resolverRepo interface {
	GetUserByShortID(ctx context.Context, shortID string) (repository.User, error)
	GetPrimaryHostByUserID(ctx context.Context, userID string) (repository.Host, error)
}

// RepoResolver implements ContainerResolver using the database repository.
// It validates the user's entry_password, checks that the user is active
// and has a running host, then resolves the container's SSH address via
// Docker inspect.
type RepoResolver struct {
	repo resolverRepo
}

func NewRepoResolver(repo resolverRepo) *RepoResolver {
	return &RepoResolver{repo: repo}
}

func (r *RepoResolver) ResolveContainer(ctx context.Context, shortID, password string) (string, error) {
	user, err := r.repo.GetUserByShortID(ctx, shortID)
	if err != nil {
		return "", fmt.Errorf("user not found")
	}

	if user.Status != "active" {
		return "", fmt.Errorf("user suspended")
	}

	if user.EntryPassword == "" || subtle.ConstantTimeCompare([]byte(user.EntryPassword), []byte(password)) != 1 {
		return "", fmt.Errorf("invalid credentials")
	}

	host, err := r.repo.GetPrimaryHostByUserID(ctx, user.ID)
	if err != nil {
		return "", fmt.Errorf("no host available")
	}

	if host.Status != "running" {
		return "", fmt.Errorf("host not running (status: %s)", host.Status)
	}

	containerName := fmt.Sprintf("cloudproxy-%s", host.ID)
	containerIP, err := getContainerIP(ctx, containerName)
	if err != nil {
		return "", fmt.Errorf("cannot resolve container address: %w", err)
	}

	return fmt.Sprintf("%s:22", containerIP), nil
}

func getContainerIP(ctx context.Context, containerName string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", "inspect", "-f",
		"{{range .NetworkSettings.Networks}}{{.IPAddress}} {{end}}", containerName)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("docker inspect: %w", err)
	}
	ips := strings.Fields(strings.TrimSpace(string(out)))
	if len(ips) == 0 {
		return "", fmt.Errorf("no IP found for container %s", containerName)
	}
	return ips[len(ips)-1], nil
}
