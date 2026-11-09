package daemon

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/resources"
)

// Resource manifests describe only a consumer's selected resources. Global configuration
// identity and source positions must not make unrelated edits reload a running integration.
func resourceManifest(snapshot *config.Snapshot, content resources.Set) (*v1.ResourceManifest, error) {
	resolved := resolveMessage(snapshot)
	manifest := &v1.ResourceManifest{
		Snapshot: resolved.Snapshot, Settings: resolved.Settings, Diagnostics: resolved.Diagnostics,
	}
	skills := map[string]resources.Skill{}
	for _, skill := range content.Skills {
		skills[skill.ID] = skill
	}
	for i, resource := range snapshot.Resources {
		var digest string
		switch resource.Kind {
		case "skill":
			definition := snapshot.Config.Skills[resource.ID]
			definition.Source = config.Source{}
			skill := skills[string(resource.ID)]
			digest = config.Digest([]any{definition, skill.Name, skill.Digest})
		case "instructions":
			definition := snapshot.Config.Instructions[resource.ID]
			definition.Source = config.Source{}
			digest = config.Digest([]any{definition, snapshot.InstructionHashes[resource.ID]})
		case "mcp":
			var err error
			digest, err = resourceMCPDigest(snapshot.Config, resource.ID)
			if err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("unsupported resource kind %s", resource.Kind)
		}
		manifest.Resources = append(manifest.Resources, &v1.ResourceManifestEntry{
			Resource: resolved.Snapshot.Resources[i], Digest: digest,
		})
	}
	settings := []*v1.HarnessSetting{}
	for _, setting := range manifest.Settings {
		settings = append(settings, &v1.HarnessSetting{Path: setting.Path, ValueJson: setting.ValueJson})
	}
	manifest.Revision = config.Digest(struct {
		Directory, Harness string
		Resources          []*v1.ResourceManifestEntry
		Environment        []string
		Settings           []*v1.HarnessSetting
		Missing            []config.ID
	}{snapshot.Directory, snapshot.Harness, manifest.Resources, snapshot.RequiredEnvironment, settings, snapshot.Missing})
	manifest.Snapshot.Id = manifest.Revision
	return manifest, nil
}

// Include only the selected server's secret definitions and file injections, never resolved
// credentials or the other servers and secrets in the configuration.
func resourceMCPDigest(c *config.Config, id config.ID) (string, error) {
	definition := c.MCP[id]
	definition.Source = config.Source{}
	secrets := map[config.ID]config.Secret{}
	files := map[string]string{}
	var valueFile func(config.Value) error
	var secretDefinition func(config.ID) error
	valueFile = func(value config.Value) error {
		if value.File != "" {
			data, err := value.Resolve("resource input file", nil, nil)
			if err != nil {
				return fmt.Errorf("MCP %s: input file unavailable", id)
			}
			files[value.File] = config.Digest(data)
		}
		if value.Secret != "" {
			return secretDefinition(value.Secret)
		}
		return nil
	}
	secretDefinition = func(id config.ID) error {
		if _, exists := secrets[id]; exists {
			return nil
		}
		secret := c.Secrets[id]
		secret.Source = config.Source{}
		secrets[id] = secret
		return valueFile(secret.AgeKey)
	}
	if err := valueFile(definition.Auth.Value); err != nil {
		return "", err
	}
	for _, injection := range definition.Injections(id) {
		if err := valueFile(injection.Value); err != nil {
			return "", err
		}
	}
	return config.Digest([]any{definition, secrets, files}), nil
}

// Polling keeps resource detection daemon-owned, including local content that config.Inputs
// does not track. Each stream resolves one scope at most once per interval.
const resourceWatchInterval = time.Second

func (s *Server) WatchResources(
	ctx context.Context,
	req *connect.Request[v1.WatchResourcesRequest],
	stream *connect.ServerStream[v1.WatchResourcesResponse],
) error {
	if !equalBearer(req.Header().Get("Authorization"), s.discovery.Token) {
		return rpcError(connect.CodeUnauthenticated, "management credential required")
	}
	previous := ""
	ticker := time.NewTicker(resourceWatchInterval)
	defer ticker.Stop()
	for {
		message := &v1.WatchResourcesResponse{}
		snapshot, err := s.resourceSnapshot(req.Msg.Directory, req.Msg.Harness)
		var manifest *v1.ResourceManifest
		if err == nil {
			var content resources.Set
			content, err = snapshot.Content()
			if err == nil {
				manifest, err = resourceManifest(snapshot, content)
			}
		}
		state := "invalid"
		if err != nil {
			message.State = &v1.WatchResourcesResponse_Invalid{Invalid: &v1.ResourceWatchInvalid{
				Diagnostics: []*v1.Diagnostic{
					diagnostic(
						"invalid_resources",
						"Resources could not be resolved; retaining the last valid resources. Run agx config validate.",
					),
				},
			}}
		} else {
			state = manifest.Revision
			message.State = &v1.WatchResourcesResponse_Manifest{Manifest: manifest}
		}
		if state != previous {
			if err := stream.Send(message); err != nil {
				return err
			}
			previous = state
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.drain.stopping.Done():
			return refusal()
		case <-ticker.C:
		}
	}
}
