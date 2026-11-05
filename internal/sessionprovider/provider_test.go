package sessionprovider

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
)

func TestCatalogScopesAndOfflineHistory(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "cache.db")
	catalog := New(path)
	alpha := &v1.SessionSource{
		SourceId: "profile",
		ScopeId:  "alpha",
		Epoch:    "one",
		Builtin:  "opencode",
		Harness:  "opencode-work",
	}
	beta := proto.Clone(alpha).(*v1.SessionSource)
	beta.ScopeId = "beta"
	for _, source := range []*v1.SessionSource{alpha, beta} {
		connection, err := catalog.Connect(source)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { catalog.Disconnect(connection) })
		if err := catalog.Publish(
			ctx,
			&v1.PublishSessionsRequest{
				Source:   source,
				Replace:  true,
				Sessions: []*v1.ProviderSession{testSession("same-native-id", source.ScopeId)},
			},
		); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := catalog.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ScopeID != "beta" {
		t.Fatalf("one native session was duplicated across locations: %+v", entries)
	}
	if err := catalog.Publish(ctx, &v1.PublishSessionsRequest{Source: alpha, Replace: true}); err != nil {
		t.Fatal(err)
	}
	entries, err = catalog.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ScopeID != "beta" || !entries[0].Online {
		t.Fatalf("scope replacement changed another location: %+v", entries)
	}
	// A replacement daemon reconstructs history without any connected native integration.
	entries, err = New(path).List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ID != "same-native-id" || entries[0].Harness != "opencode-work" ||
		entries[0].Online {
		t.Fatalf("offline catalog did not retain native history: %+v", entries)
	}
}

func TestSupersededEpochCannotPublishOrReconnect(t *testing.T) {
	catalog := New(filepath.Join(t.TempDir(), "cache.db"))
	oldSource := &v1.SessionSource{
		SourceId: "profile",
		ScopeId:  "location",
		Epoch:    "old",
		Builtin:  "opencode",
		Harness:  "opencode",
	}
	old, err := catalog.Connect(oldSource)
	if err != nil {
		t.Fatal(err)
	}
	reconnected, err := catalog.Connect(proto.Clone(oldSource).(*v1.SessionSource))
	if err != nil {
		t.Fatalf("same instance could not replace its disconnected stream: %v", err)
	}
	select {
	case <-old.Done:
	default:
		t.Fatal("same-epoch reconnect left the old command stream active")
	}
	catalog.Disconnect(old)
	select {
	case <-reconnected.Done:
		t.Fatal("superseded stream cleanup disconnected its same-epoch replacement")
	default:
	}
	if err := catalog.Publish(
		t.Context(),
		&v1.PublishSessionsRequest{
			Source:   oldSource,
			Sessions: []*v1.ProviderSession{testSession("session", "location")},
		},
	); err != nil {
		t.Fatalf("same-epoch replacement could not publish: %v", err)
	}
	catalog.Disconnect(reconnected)
	newSource := proto.Clone(oldSource).(*v1.SessionSource)
	newSource.Epoch = "new"
	current, err := catalog.Connect(newSource)
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Disconnect(current)
	if err := catalog.Publish(
		t.Context(),
		&v1.PublishSessionsRequest{Source: oldSource, Replace: true},
	); !errors.Is(
		err,
		ErrStale,
	) {
		t.Fatalf("stale publisher accepted: %v", err)
	}
	if _, err := catalog.Connect(oldSource); !errors.Is(err, ErrStale) {
		t.Fatalf("stale epoch reclaimed current scope: %v", err)
	}
	catalog.Disconnect(reconnected)
	if err := catalog.Publish(t.Context(), &v1.PublishSessionsRequest{Source: newSource, Replace: true}); err != nil {
		t.Fatalf("stale disconnect offlined successor: %v", err)
	}
}

func TestMovedSessionKeepsNewestSummary(t *testing.T) {
	ctx := t.Context()
	catalog := New(filepath.Join(t.TempDir(), "cache.db"))
	alpha := &v1.SessionSource{
		SourceId: "profile",
		ScopeId:  "alpha",
		Epoch:    "one",
		Builtin:  "opencode",
		Harness:  "opencode",
	}
	beta := proto.Clone(alpha).(*v1.SessionSource)
	beta.ScopeId = "beta"
	for _, source := range []*v1.SessionSource{alpha, beta} {
		connection, err := catalog.Connect(source)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { catalog.Disconnect(connection) })
	}
	original := testSession("moving", "alpha")
	if err := catalog.Publish(
		ctx,
		&v1.PublishSessionsRequest{Source: alpha, Replace: true, Sessions: []*v1.ProviderSession{original}},
	); err != nil {
		t.Fatal(err)
	}
	moved := proto.Clone(original).(*v1.ProviderSession)
	moved.Directory, moved.Title = "beta", "Moved title"
	moved.UpdatedAt = timestamppb.New(time.Unix(3, 0))
	if err := catalog.Publish(
		ctx,
		&v1.PublishSessionsRequest{Source: beta, Sessions: []*v1.ProviderSession{moved}},
	); err != nil {
		t.Fatal(err)
	}
	for _, publication := range []*v1.PublishSessionsRequest{
		{Source: alpha, Replace: true, Sessions: []*v1.ProviderSession{original}},
		{Source: beta, Sessions: []*v1.ProviderSession{original}},
		{Source: beta, Replace: true, Sessions: []*v1.ProviderSession{original}},
		{Source: alpha, DeletedIds: []string{original.Id}},
		{Source: alpha, Replace: true},
	} {
		if err := catalog.Publish(ctx, publication); err != nil {
			t.Fatal(err)
		}
		entries, err := catalog.List(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 || entries[0].ScopeID != "beta" || entries[0].Directory != "beta" ||
			entries[0].Title != moved.Title ||
			!entries[0].Updated.Equal(moved.UpdatedAt.AsTime()) {
			t.Fatalf("stale publication or old-location cleanup changed a moved session: %+v", entries)
		}
	}
	otherProfile := proto.Clone(beta).(*v1.SessionSource)
	otherProfile.SourceId = "another-profile"
	connection, err := catalog.Connect(otherProfile)
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Disconnect(connection)
	if err := catalog.Publish(
		ctx,
		&v1.PublishSessionsRequest{Source: otherProfile, Sessions: []*v1.ProviderSession{original}},
	); err != nil {
		t.Fatal(err)
	}
	entries, err := catalog.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].SourceID == entries[1].SourceID {
		t.Fatalf("the same native ID collided between native profiles: %+v", entries)
	}
}

func testSession(id, directory string) *v1.ProviderSession {
	return &v1.ProviderSession{
		Id:        id,
		Directory: directory,
		Title:     "Native title",
		StartedAt: timestamppb.New(time.Unix(1, 0)),
		UpdatedAt: timestamppb.New(time.Unix(2, 0)),
	}
}
