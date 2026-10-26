package daemon

import (
	"context"
	"os"
	"regexp"

	"connectrpc.com/connect"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/packages"
)

// Install streams an installation's progress and then its result or failure. The installation
// goes on if the caller disconnects; a stopping daemon waits for it for its grace period, then
// cancels it.
func (s *Server) Install(
	ctx context.Context,
	req *connect.Request[v1.InstallRequest],
	stream *connect.ServerStream[v1.InstallResponse],
) error {
	frozen := req.Msg.Frozen
	done := make(chan *v1.InstallResponse, 1)
	s.mu.Lock()
	if s.installsClosed {
		s.mu.Unlock()
		return refusal()
	}
	s.installs.Go(func() {
		done <- s.install(frozen)
	})
	s.mu.Unlock()
	if err := stream.Send(&v1.InstallResponse{
		State: &v1.InstallResponse_Progress{Progress: &v1.PackageProgress{}},
	}); err != nil {
		return err
	}
	select {
	case response := <-done:
		return stream.Send(response)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Server) install(frozen bool) *v1.InstallResponse {
	result, err := packages.Install(
		s.ctx,
		s.paths.Config,
		s.paths.Cache,
		s.builtins,
		packages.Options{Frozen: frozen},
	)
	if err == nil && !frozen {
		var c *config.Config
		c, err = config.Load(s.paths.Config, config.Options{Builtins: s.builtins, Cache: s.paths.Cache})
		if err == nil {
			// Skipped installations need no action from the requester; launches warn about them.
			_, err = s.adapters.Install(s.ctx, c, s.paths.Cache, os.LookupEnv)
		}
	}
	if err != nil {
		return &v1.InstallResponse{State: &v1.InstallResponse_Failure{
			Failure: diagnostic("install_failed", installFailure(err)),
		}}
	}
	names := []string{}
	for _, id := range result.PackageIDs {
		names = append(names, string(id))
	}
	return &v1.InstallResponse{State: &v1.InstallResponse_Result{
		Result: &v1.PackageResult{PackageIds: names, Lockfiles: result.Lockfiles},
	}}
}

// urlCredentials matches the user information of URLs, such as a token in a Git remote, which
// git's errors may repeat.
var urlCredentials = regexp.MustCompile(`(://)[^/@\s]+@`)

// installFailure says why an installation failed, without credentials a URL in it carries.
func installFailure(err error) string {
	return "Package installation failed: " + urlCredentials.ReplaceAllString(err.Error(), "${1}***@")
}
