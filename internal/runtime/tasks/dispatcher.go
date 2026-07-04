package tasks

import (
	"context"
	"fmt"

	"github.com/claudedock/claudedock/internal/agentapi"
)

type Dispatcher struct {
	client interface {
		RunHostAction(context.Context, agentapi.HostActionRequest) (agentapi.HostActionResponse, error)
	}
}

func NewDispatcher(client interface {
	RunHostAction(context.Context, agentapi.HostActionRequest) (agentapi.HostActionResponse, error)
}) *Dispatcher {
	return &Dispatcher{client: client}
}

func (d *Dispatcher) Dispatch(ctx context.Context, request agentapi.HostActionRequest) (agentapi.HostActionResponse, error) {
	switch request.Action {
	case agentapi.ActionCreateHost, agentapi.ActionStartHost, agentapi.ActionStopHost, agentapi.ActionRebuildHost, agentapi.ActionPrepareHost:
	default:
		return agentapi.HostActionResponse{}, fmt.Errorf("unsupported host action: %s", request.Action)
	}

	response, err := d.client.RunHostAction(ctx, request)
	if err != nil {
		return agentapi.HostActionResponse{}, fmt.Errorf("dispatch host action: %w", err)
	}

	return response, nil
}
