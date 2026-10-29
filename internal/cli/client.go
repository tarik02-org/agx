package cli

import (
	"context"
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
}

func newClient(paths platform.Paths) (*client, error) {
	d, err := daemon.ReadDiscovery(paths)
	if err != nil {
		return nil, err
	}
	return dial(d), nil
}

func dial(d daemon.Discovery) *client {
	httpClient := &http.Client{
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
	options := connect.WithInterceptors(daemonInterceptor{token: d.Token})
	return &client{
		discovery: d,
		config:    agxv1connect.NewConfigurationServiceClient(httpClient, d.URL, options),
		launches:  agxv1connect.NewLaunchServiceClient(httpClient, d.URL, options),
		runtime:   agxv1connect.NewRuntimeServiceClient(httpClient, d.URL, options),
		secrets:   agxv1connect.NewSecretServiceClient(httpClient, d.URL, options),
	}
}

// withToken authorizes a call with a token other than the daemon's, such as a launch's lifecycle token.
func withToken[T any](msg *T, token string) *connect.Request[T] {
	r := connect.NewRequest(msg)
	r.Header().Set("Authorization", "Bearer "+token)
	return r
}

// daemonInterceptor authorizes calls that carry no token of their own with the daemon's, and
// bounds unary calls without a deadline by callTimeout. Streams, such as a login waiting for the
// browser, run as long as they need.
type daemonInterceptor struct{ token string }

func (i daemonInterceptor) authorize(header http.Header) {
	if header.Get("Authorization") == "" {
		header.Set("Authorization", "Bearer "+i.token)
	}
}

func (i daemonInterceptor) WrapUnary(next connect.UnaryFunc) connect.UnaryFunc {
	return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
		i.authorize(req.Header())
		if _, ok := ctx.Deadline(); !ok {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, callTimeout)
			defer cancel()
		}
		return next(ctx, req)
	}
}

func (i daemonInterceptor) WrapStreamingClient(next connect.StreamingClientFunc) connect.StreamingClientFunc {
	return func(ctx context.Context, spec connect.Spec) connect.StreamingClientConn {
		conn := next(ctx, spec)
		i.authorize(conn.RequestHeader())
		return conn
	}
}

func (daemonInterceptor) WrapStreamingHandler(next connect.StreamingHandlerFunc) connect.StreamingHandlerFunc {
	return next
}
