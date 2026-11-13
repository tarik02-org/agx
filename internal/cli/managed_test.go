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

	"github.com/gofrs/flock"

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
	holdStartupLock(t, paths)
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

// holdStartupLock holds the lock a serving daemon holds, for a test server standing in for one.
func holdStartupLock(t *testing.T, paths platform.Paths) {
	t.Helper()
	lock := flock.New(filepath.Join(paths.Runtime, "daemon.lock"))
	if ok, err := lock.TryLock(); err != nil || !ok {
		t.Fatalf("startup lock: %v %v", ok, err)
	}
	t.Cleanup(func() { _ = lock.Unlock() })
}

func TestDiscoveryNeedsTheStartupLock(t *testing.T) {
	paths := platform.Paths{Runtime: t.TempDir()}
	discovery := control.Discovery{URL: "http://127.0.0.1:4000", Token: "token", InstanceID: "crashed"}
	if err := securefs.WritePrivateJSON(filepath.Join(paths.Runtime, "daemon.json"), discovery); err != nil {
		t.Fatal(err)
	}
	if _, err := newClient(paths); !errors.Is(err, control.ErrStale) || !daemonGone(err) {
		t.Fatalf("discovery a crashed daemon left = %v", err)
	}
	l := &lease{paths: paths, client: dial(discovery)}
	called := false
	err := l.call(func(*client) error { called = true; return nil })
	if called || !daemonGone(err) || connect.CodeOf(err) != connect.CodeUnavailable {
		t.Fatalf("lease called the port of a crashed daemon: %v, %v", called, err)
	}
	holdStartupLock(t, paths)
	if c, err := newClient(paths); err != nil || c.discovery.InstanceID != "crashed" {
		t.Fatalf("discovery of the daemon holding the lock = %v, %v", c, err)
	}
}
