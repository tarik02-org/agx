package daemon

import (
	"time"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/config"
	"github.com/tarik02-org/agx/internal/mcpruntime"
	"github.com/tarik02-org/agx/internal/resources"
)

// Polling keeps change detection daemon-owned, including skill and instruction files that
// config.Inputs does not track. An edit committed through this daemon applies at once.
const reconcileInterval = time.Second

const invalidRevision = "invalid"

// reconcileLaunches keeps every running launch on its directory's current configuration until
// the daemon stops.
func (s *Server) reconcileLaunches() {
	ticker := time.NewTicker(reconcileInterval)
	defer ticker.Stop()
	for {
		// Taken before reconciling, so an edit committed meanwhile is not missed.
		edited := s.edited.next()
		s.mu.Lock()
		launches := []*launch{}
		for _, l := range s.launches {
			if active(l.view.State) && !l.threadServer {
				launches = append(launches, l)
			}
		}
		s.mu.Unlock()
		for _, l := range launches {
			s.reconcileLaunch(l)
		}
		select {
		case <-s.ctx.Done():
			return
		// A stopping daemon hands its launches to the next one, which reconciles them.
		case <-s.drain.stopping.Done():
			return
		case <-ticker.C:
		case <-edited:
		}
	}
}

// reconcileLaunch brings launch l's MCP servers to its directory's current selection: a server
// whose definition, share root, secret definitions and inputs are unchanged keeps its endpoint,
// a changed one restarts behind the same URL, a deselected one closes, and a newly selected one
// starts. A resource lease also takes the directory's current content. A process launch keeps
// the snapshot its harness loaded, so its view still names it; only its MCP servers follow.
// Configuration that cannot be resolved leaves the launch as it is.
func (s *Server) reconcileLaunch(l *launch) {
	s.reconcileMu.Lock()
	defer s.reconcileMu.Unlock()
	resource := l.view.Kind == v1.Launch_KIND_RESOURCES
	c, err := s.reload()
	var snapshot *config.Snapshot
	if err == nil && resource {
		snapshot, err = resolveResources(c, l.view.Directory, l.view.Harness)
	} else if err == nil {
		snapshot, err = c.Resolve(l.view.Directory, l.view.Harness)
	}
	directory := l.view.Directory
	if resource && err == nil {
		directory, err = resourceMCPDirectory(snapshot, l.mcpDirectory)
	}
	var content resources.Set
	if resource && err == nil {
		content, err = snapshot.Content()
	}
	if err != nil {
		if resource {
			s.mu.Lock()
			if active(l.view.State) && l.revision != invalidRevision {
				l.revision = invalidRevision
				s.changed.notify()
			}
			s.mu.Unlock()
		}
		return
	}
	s.mu.Lock()
	current, inputs := l.endpoints, l.inputs
	s.mu.Unlock()
	endpoints := map[config.ID]*mcpruntime.Endpoint{}
	stale, fresh := map[config.ID]*mcpruntime.Endpoint{}, map[config.ID]*mcpruntime.Endpoint{}
	for _, r := range snapshot.Resources {
		if r.Kind != "mcp" {
			continue
		}
		definition, root, own := endpointBasis(snapshot, r.ID, inputs, directory)
		e := current[r.ID]
		if e == nil || e.Basis() != mcpruntime.Basis(r.ID, definition, root, snapshot.Config.Secrets, own) {
			e = mcpruntime.New(l.ctx, r.ID, definition, snapshot.Config.Secrets, own, s.manager, s.shared, root)
			fresh[r.ID] = e
		}
		endpoints[r.ID] = e
	}
	for id, e := range current {
		if endpoints[id] != e {
			stale[id] = e
		}
	}
	var leased *v1.LeaseResources
	revision := ""
	if resource {
		leased, revision = s.leaseResources(l.view.Id, snapshot, content, endpoints)
	}
	s.mu.Lock()
	// end closes the endpoints the launch holds, so a launch that ended meanwhile closes these.
	ended := !active(l.view.State)
	changed := false
	if !ended {
		if len(stale) > 0 || len(fresh) > 0 {
			l.endpoints, l.snapshot = endpoints, snapshot
			changed = true
		}
		if len(stale) > 0 {
			s.clients.closeLaunch(l.view.Id)
		}
		if resource && revision != l.revision {
			l.snapshot, l.resources, l.revision = snapshot, leased, revision
			l.view.SnapshotId = snapshot.ID
			changed = true
		}
		if changed {
			s.changed.notify()
		}
	}
	s.mu.Unlock()
	if ended {
		closeEndpoints(fresh)
		return
	}
	go closeEndpoints(stale)
}
