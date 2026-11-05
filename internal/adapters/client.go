package adapters

// ClientLauncher starts a native client whose integration owns resource delivery.
// ClientExecutable keeps this capability distinct from managed process launchers.
type ClientLauncher interface {
	Adapter
	Resumer
	Modes() []string
	Classify(args []string) string
	ClientExecutable(configured string) (string, error)
}

func (r Registry) ClientLauncher(name string) (ClientLauncher, bool) {
	adapter, ok := r.adapters[name].(ClientLauncher)
	return adapter, ok
}
