package cli

import (
	"context"
	"errors"
	"net/http"
	"time"

	"connectrpc.com/connect"

	"github.com/tarik02-org/agx/gen/agx/v1/agxv1connect"
	"github.com/tarik02-org/agx/internal/daemon"
	"github.com/tarik02-org/agx/internal/platform"
)

const (
	callTimeout = 20 * time.Second
	// Preparing a launch may compile and install snapshot artifacts.
	launchTimeout = 2 * time.Minute
)

type client struct {
	discovery daemon.Discovery
	config    agxv1connect.ConfigurationServiceClient
	launches  agxv1connect.LaunchServiceClient
	runtime   agxv1connect.RuntimeServiceClient
	secrets   agxv1connect.SecretServiceClient
	ui        agxv1connect.UIServiceClient
	http      *http.Client
}

func newClient(paths platform.Paths) (*client, error) {
	d, err := daemon.ReadDiscovery(paths)
	if err != nil {
		return nil, err
	}
	return dial(d), nil
}

// newAnyVersionClient reaches a daemon of another version so it can still be detected and stopped.
func newAnyVersionClient(paths platform.Paths) (*client, error) {
	d, err := daemon.ReadDiscovery(paths)
	if err != nil && !errors.Is(err, daemon.ErrVersionMismatch) {
		return nil, err
	}
	return dial(d), nil
}

// Timeouts are per call: a client-wide timeout would also cut off streams and slow operations such as login.
func dial(d daemon.Discovery) *client {
	httpClient := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
	return &client{
		discovery: d,
		http:      httpClient,
		config:    agxv1connect.NewConfigurationServiceClient(httpClient, d.URL),
		launches:  agxv1connect.NewLaunchServiceClient(httpClient, d.URL),
		runtime:   agxv1connect.NewRuntimeServiceClient(httpClient, d.URL),
		secrets:   agxv1connect.NewSecretServiceClient(httpClient, d.URL),
		ui:        agxv1connect.NewUIServiceClient(httpClient, d.URL),
	}
}

func request[T any](msg *T, token string) *connect.Request[T] {
	r := connect.NewRequest(msg)
	r.Header().Set("Authorization", "Bearer "+token)
	return r
}

func boundedContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}
