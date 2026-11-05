package sessionprovider

import (
	"context"
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
	if len(entries) != 1 || entries[0].ID != "same-native-id" || entries[0].Online {
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
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	interrupted := make(chan error, 1)
	go func() {
		_, err := catalog.Prepare(
			ctx,
			&v1.PrepareSessionRequest{
				SourceId:        oldSource.SourceId,
				ScopeId:         oldSource.ScopeId,
				NativeSessionId: "waiting",
			},
		)
		interrupted <- err
	}()
	var abandoned *v1.SessionCommand
	select {
	case abandoned = <-old.Commands:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
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
	select {
	case err := <-interrupted:
		if !errors.Is(err, ErrOffline) {
			t.Fatalf("command on the superseded stream was not canceled: %v", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if err := catalog.Reply(
		&v1.ReplySessionCommandRequest{
			Source:    oldSource,
			CommandId: abandoned.Id,
			Result: &v1.ReplySessionCommandRequest_Prepared{
				Prepared: &v1.PreparedSession{Session: testSession("waiting", "location")},
			},
		},
	); !errors.Is(
		err,
		ErrNotFound,
	) {
		t.Fatalf("late reply from the replaced stream was accepted: %v", err)
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

func TestPreparePreservesProviderFailureKinds(t *testing.T) {
	for _, tc := range []struct {
		name string
		code v1.SessionCommandFailure_Code
		want error
	}{
		{"missing", v1.SessionCommandFailure_CODE_NOT_FOUND, ErrNotFound},
		{"unavailable", v1.SessionCommandFailure_CODE_UNAVAILABLE, ErrOffline},
		{"rejected", v1.SessionCommandFailure_CODE_REJECTED, ErrRejected},
		{"unspecified", v1.SessionCommandFailure_CODE_UNSPECIFIED, ErrInvalidReply},
		{"unknown", v1.SessionCommandFailure_Code(99), ErrInvalidReply},
	} {
		t.Run(tc.name, func(t *testing.T) {
			catalog := New(filepath.Join(t.TempDir(), "cache.db"))
			source := &v1.SessionSource{
				SourceId: "profile",
				ScopeId:  "location",
				Epoch:    "one",
				Builtin:  "opencode",
				Harness:  "opencode",
			}
			connection, err := catalog.Connect(source)
			if err != nil {
				t.Fatal(err)
			}
			defer catalog.Disconnect(connection)
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			result := make(chan error, 1)
			go func() {
				response, err := catalog.Prepare(
					ctx,
					&v1.PrepareSessionRequest{
						SourceId:        source.SourceId,
						ScopeId:         source.ScopeId,
						NativeSessionId: "session",
					},
				)
				if response != nil {
					result <- errors.New("provider failure produced a prepared session")
					return
				}
				result <- err
			}()
			var command *v1.SessionCommand
			select {
			case command = <-connection.Commands:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if err := catalog.Reply(
				&v1.ReplySessionCommandRequest{
					Source:    source,
					CommandId: command.Id,
					Result: &v1.ReplySessionCommandRequest_Failure{
						Failure: &v1.SessionCommandFailure{Code: tc.code, Message: "private native failure detail"},
					},
				},
			); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-result:
				if !errors.Is(err, tc.want) {
					t.Fatalf("failure %s became %v, want %v", tc.name, err, tc.want)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		})
	}
}

func TestPrepareRequiresMatchingNativeValidation(t *testing.T) {
	catalog := New(filepath.Join(t.TempDir(), "cache.db"))
	source := &v1.SessionSource{
		SourceId: "profile",
		ScopeId:  "location",
		Epoch:    "one",
		Builtin:  "opencode",
		Harness:  "opencode",
	}
	connection, err := catalog.Connect(source)
	if err != nil {
		t.Fatal(err)
	}
	defer catalog.Disconnect(connection)
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	type prepared struct {
		response *v1.PrepareSessionResponse
		err      error
	}
	result := make(chan prepared, 1)
	go func() {
		response, err := catalog.Prepare(
			ctx,
			&v1.PrepareSessionRequest{SourceId: "profile", ScopeId: "location", NativeSessionId: "wanted"},
		)
		result <- prepared{response, err}
	}()
	var command *v1.SessionCommand
	select {
	case command = <-connection.Commands:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	wrong := &v1.ReplySessionCommandRequest{
		Source:    source,
		CommandId: command.Id,
		Result: &v1.ReplySessionCommandRequest_Prepared{
			Prepared: &v1.PreparedSession{
				Session:        testSession("different", "location"),
				LaunchDataJson: "private-routing",
			},
		},
	}
	if err := catalog.Reply(wrong); !errors.Is(err, ErrNotFound) {
		t.Fatalf("mismatched native validation accepted: %v", err)
	}
	wrong.GetPrepared().Session.Id = "wanted"
	if err := catalog.Reply(wrong); err != nil {
		t.Fatal(err)
	}
	select {
	case reply := <-result:
		if reply.err != nil || reply.response.Session.Id != "wanted" ||
			reply.response.LaunchDataJson != "private-routing" {
			t.Fatalf("unexpected preparation: %+v", reply)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	entries, err := catalog.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("private preparation was persisted: %+v", entries)
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
