package docker

import (
	"context"
	"fmt"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// manager wraps docker client operations for container control
type Manager struct {
	cli *client.Client
}

// containerinfo holds status details of a docker container
type ContainerInfo struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	State     string `json:"state"`
	Running   bool   `json:"running"`
	StartedAt string `json:"started_at"`
	Status    string `json:"status"`
}

// newmanager creates a new docker client manager using environment configuration
func NewManager() (*Manager, error) {
	// initialize docker client with environment configuration
	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}

	return &Manager{cli: cli}, nil
}

// close closes the docker client
func (m *Manager) Close() error {
	return m.cli.Close()
}

// getstatus inspects a container and returns detailed info
func (m *Manager) GetStatus(ctx context.Context, containerID string) (*ContainerInfo, error) {
	// inspect container by name or id
	inspect, err := m.cli.ContainerInspect(ctx, containerID)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container %s: %w", containerID, err)
	}

	idShort := inspect.ID
	if len(idShort) > 12 {
		idShort = idShort[:12]
	}

	info := &ContainerInfo{
		ID:        idShort,
		Name:      inspect.Name,
		State:     inspect.State.Status,
		Running:   inspect.State.Running,
		StartedAt: inspect.State.StartedAt,
		Status:    inspect.State.Status,
	}

	return info, nil
}

// startcontainer starts a stopped docker container
func (m *Manager) StartContainer(ctx context.Context, containerID string) (*ContainerInfo, error) {
	// start container
	err := m.cli.ContainerStart(ctx, containerID, container.StartOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to start container %s: %w", containerID, err)
	}

	// fetch updated status after start
	return m.GetStatus(ctx, containerID)
}

// stopcontainer stops a running docker container
func (m *Manager) StopContainer(ctx context.Context, containerID string) (*ContainerInfo, error) {
	// stop container with 10 second timeout
	timeout := 10
	stopOpts := container.StopOptions{
		Timeout: &timeout,
	}

	err := m.cli.ContainerStop(ctx, containerID, stopOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to stop container %s: %w", containerID, err)
	}

	// fetch updated status after stop
	return m.GetStatus(ctx, containerID)
}

// executeaction executes requested action (start, stop, status) on target container
func (m *Manager) ExecuteAction(ctx context.Context, action, containerID string) (*ContainerInfo, error) {
	switch action {
	case "start":
		return m.StartContainer(ctx, containerID)
	case "stop":
		return m.StopContainer(ctx, containerID)
	case "status":
		return m.GetStatus(ctx, containerID)
	default:
		return nil, fmt.Errorf("unsupported action: %s", action)
	}
}
