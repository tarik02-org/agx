package platform

import (
	"os/exec"
	"sync"
)

// Group is a started command together with the descendants it spawns.
type Group struct {
	cmd    *exec.Cmd
	mu     sync.Mutex
	reaped bool
	group
}

// Start starts cmd in a group of its own that Kill ends as a whole.
func Start(cmd *exec.Cmd) (*Group, error) {
	g := &Group{cmd: cmd}
	if err := g.start(); err != nil {
		return nil, err
	}
	return g, nil
}

// Wait waits for the command like exec.Cmd.Wait; on Windows it then ends the descendants left behind.
func (g *Group) Wait() error {
	err := g.cmd.Wait()
	g.mu.Lock()
	defer g.mu.Unlock()
	g.reaped = true
	g.release()
	return err
}

// Kill ends the command and its descendants. Once Wait reaped the command, Kill does nothing:
// on Unix its process group ID may then belong to an unrelated group.
func (g *Group) Kill() { g.whileUnreaped(g.kill) }

// Terminate asks the command's group to exit where the platform has such a request.
func (g *Group) Terminate() { g.whileUnreaped(g.terminate) }

func (g *Group) whileUnreaped(f func()) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.reaped {
		f()
	}
}
