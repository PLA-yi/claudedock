package agentapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

const DefaultSocketPath = "/run/claudedock/host-agent.sock"

type Client struct {
	socketPath string
	httpClient *http.Client
}

func NewClient(socketPath string) *Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
		},
	}

	return &Client{
		socketPath: socketPath,
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
	}
}

func (c *Client) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://host-agent/healthz", nil)
	if err != nil {
		return fmt.Errorf("agent unreachable: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("agent unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("agent unhealthy: status %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) InspectContainer(ctx context.Context, containerName string) (ContainerStatusResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("http://host-agent/v1/containers/%s/status", containerName), nil)
	if err != nil {
		return ContainerStatusResponse{}, fmt.Errorf("build inspect request: %w", err)
	}

	response, err := c.httpClient.Do(req)
	if err != nil {
		return ContainerStatusResponse{}, fmt.Errorf("inspect container via %s: %w", c.socketPath, err)
	}
	defer response.Body.Close()

	var payload ContainerStatusResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return ContainerStatusResponse{}, fmt.Errorf("decode inspect response: %w", err)
	}

	return payload, nil
}

func (c *Client) RunHostAction(ctx context.Context, request HostActionRequest) (HostActionResponse, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return HostActionResponse{}, fmt.Errorf("marshal host action request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://host-agent/v1/host-actions", bytes.NewReader(body))
	if err != nil {
		return HostActionResponse{}, fmt.Errorf("build host action request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	response, err := c.httpClient.Do(req)
	if err != nil {
		return HostActionResponse{}, fmt.Errorf("run host action via %s: %w", c.socketPath, err)
	}
	defer response.Body.Close()

	var payload HostActionResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return HostActionResponse{}, fmt.Errorf("decode host action response: %w", err)
	}

	if response.StatusCode >= http.StatusBadRequest {
		return payload, fmt.Errorf("host action %s failed: %s", request.Action, payload.Update.ErrorMessage)
	}

	return payload, nil
}
