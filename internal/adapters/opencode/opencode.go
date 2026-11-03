// Package opencode identifies the native OpenCode plugin as a configuration consumer.
package opencode

type Adapter struct{}

func (Adapter) Name() string { return "opencode" }
