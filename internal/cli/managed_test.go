package cli

import (
	"context"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"connectrpc.com/connect"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/gen/agx/v1/agxv1connect"
	"github.com/tarik02-org/agx/internal/control"
	"github.com/tarik02-org/agx/internal/platform"
	"github.com/tarik02-org/agx/internal/securefs"
)

func TestAwaitManagedRequiresNewReadyInstance(t *testing.T) {
	isolate(t, "")
	paths, err := platform.DefaultPaths()
	if err != nil {
		t.Fatal(err)
	}
	if err := paths.Derive(); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(connect.NewUnaryHandler(
		agxv1connect.RuntimeServiceGetRuntimeProcedure,
		func(ctx context.Context, req *connect.Request[v1.GetRuntimeRequest]) (*connect.Response[v1.GetRuntimeResponse], error) {
			if req.Header().Get("Authorization") != "Bearer test-credential" {
				return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("wrong credential"))
			}
			return connect.NewResponse(&v1.GetRuntimeResponse{}), nil
		},
	))
	defer server.Close()
	if err := os.MkdirAll(paths.Runtime, 0o700); err != nil {
		t.Fatal(err)
	}
	discovery := control.Discovery{
		URL: server.URL, Token: "test-credential", InstanceID: "previous", Managed: true,
	}
	file := filepath.Join(paths.Runtime, "daemon.json")
	if err := securefs.WritePrivateJSON(file, discovery); err != nil {
		t.Fatal(err)
	}
	_, err = awaitManaged(t.Context(), paths, "previous", 20*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("old healthy instance satisfied restart: %v", err)
	}
	discovery.InstanceID, discovery.Token = "replacement", "wrong-credential"
	if err := securefs.WritePrivateJSON(file, discovery); err != nil {
		t.Fatal(err)
	}
	_, err = awaitManaged(t.Context(), paths, "previous", 20*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("unauthenticated replacement satisfied restart: %v", err)
	}
	discovery.Token = "test-credential"
	if err := securefs.WritePrivateJSON(file, discovery); err != nil {
		t.Fatal(err)
	}
	c, err := awaitManaged(t.Context(), paths, "previous", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if c.discovery.InstanceID != "replacement" {
		t.Fatalf("restart connected to %q", c.discovery.InstanceID)
	}
}
