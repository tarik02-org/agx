package daemon

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/structpb"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/configstore"
)

func (s *Server) GetLocalConfiguration(
	ctx context.Context,
	_ *connect.Request[v1.GetLocalConfigurationRequest],
) (*connect.Response[v1.GetLocalConfigurationResponse], error) {
	c, err := s.reload()
	var local *config.Local
	if err == nil {
		local = c.Local
	} else {
		local, err = configstore.Read(ctx, s.paths.State, s.paths.Config)
	}
	if err != nil {
		return nil, configError(err)
	}
	document, err := structpb.NewStruct(local.Document)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	out := &v1.GetLocalConfigurationResponse{
		Document: document,
		Revision: local.Revision,
		Packages: map[string]*v1.LocalPackageLock{},
	}
	for id, lock := range local.Lock.Packages {
		dependencies := make([]string, len(lock.Dependencies))
		for i, id := range lock.Dependencies {
			dependencies[i] = string(id)
		}
		out.Packages[string(id)] = &v1.LocalPackageLock{
			Path:         lock.Source.Path,
			Git:          lock.Source.Git,
			Ref:          lock.Source.Ref,
			SourceHash:   lock.Source.Hash,
			Subdir:       lock.Source.Subdir,
			Revision:     lock.Revision,
			Hash:         lock.Hash,
			Dependencies: dependencies,
		}
	}
	return connect.NewResponse(out), nil
}

func (s *Server) ResolvePassthrough(
	ctx context.Context,
	req *connect.Request[v1.ResolvePassthroughRequest],
) (*connect.Response[v1.ResolvePassthroughResponse], error) {
	c, err := s.reload()
	if err != nil {
		return nil, configError(err)
	}
	h, err := c.Harness(req.Msg.Harness)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	adapter, ok := s.adapters.Launcher(h.Builtin)
	if !ok || adapter.Classify(req.Msg.NativeArguments) != config.ModePassthrough {
		return nil, rpcError(connect.CodeInvalidArgument, "native arguments must select passthrough mode")
	}
	args, environment, err := s.resolveCommandValues(ctx, c, h, config.ModePassthrough, req.Msg.EnvironmentInputs)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&v1.ResolvePassthroughResponse{Arguments: args, Environment: environment}), nil
}

func (s *Server) ResolveClientCommand(
	ctx context.Context,
	req *connect.Request[v1.ResolveClientCommandRequest],
) (*connect.Response[v1.ResolveClientCommandResponse], error) {
	c, err := s.reload()
	if err != nil {
		return nil, configError(err)
	}
	h, err := c.Harness(req.Msg.Harness)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	adapter, ok := s.adapters.ClientLauncher(h.Builtin)
	if !ok {
		return nil, rpcError(connect.CodeInvalidArgument, "harness has no native client launcher")
	}
	args, environment, err := s.resolveCommandValues(
		ctx,
		c,
		h,
		adapter.Classify(req.Msg.NativeArguments),
		req.Msg.EnvironmentInputs,
	)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&v1.ResolveClientCommandResponse{Arguments: args, Environment: environment}), nil
}

func (s *Server) resolveCommandValues(
	ctx context.Context,
	c *config.Config,
	h config.ResolvedHarness,
	mode string,
	inputs map[string]string,
) ([]string, map[string]string, error) {
	allowed := map[string]bool{}
	for _, injection := range h.Injections() {
		if name := c.Variable(injection.Value); name != "" {
			allowed[name] = true
		}
	}
	for name := range inputs {
		if !allowed[name] {
			return nil, nil, rpcError(connect.CodeInvalidArgument, "undeclared environment input "+name)
		}
	}
	env := func(name string) (string, bool) { value, ok := inputs[name]; return value, ok }
	args, environment, err := h.Resolve(mode, env, func(id config.ID) (string, error) {
		secret := c.Secrets[id]
		if secret.Kind == "oauth" {
			return "", rpcError(connect.CodeInvalidArgument, "OAuth secrets authenticate only HTTP MCP servers")
		}
		return s.manager.Value(ctx, id, secret, c.Secrets, inputs, "")
	})
	if err != nil {
		return nil, nil, connect.NewError(connect.CodeFailedPrecondition, err)
	}
	return args, environment, nil
}
