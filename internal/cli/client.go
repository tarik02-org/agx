package cli

import (
	"context"
	"errors"
	"net/http"
	"time"

	"connectrpc.com/connect"

	"github.com/tarik02/weft/gen/weft/v1/weftv1connect"
	"github.com/tarik02/weft/internal/daemon"
	"github.com/tarik02/weft/internal/platform"
)

const (
	callTimeout = 20 * time.Second
	// Preparing a launch may compile and install snapshot artifacts.
	launchTimeout = 2 * time.Minute
	// Login waits for the daemon's OAuth metadata discovery and client registration with the provider.
	loginTimeout = 2 * time.Minute
)

type client struct {
	discovery daemon.Discovery
	config    weftv1connect.ConfigurationServiceClient
	launches  weftv1connect.LaunchServiceClient
	runtime   weftv1connect.RuntimeServiceClient
	auth      weftv1connect.AuthServiceClient
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
		config:    weftv1connect.NewConfigurationServiceClient(httpClient, d.URL),
		launches:  weftv1connect.NewLaunchServiceClient(httpClient, d.URL),
		runtime:   weftv1connect.NewRuntimeServiceClient(httpClient, d.URL),
		auth:      weftv1connect.NewAuthServiceClient(httpClient, d.URL),
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
