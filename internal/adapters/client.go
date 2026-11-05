package adapters

import "context"

// ClientLauncher starts a native client whose integration owns resource delivery. Its distinct
// methods keep process launchers from accidentally taking this path.
type ClientLauncher interface {
	Adapter
	ClientExecutable(configured string) (string, error)
	ClientArguments(args []string, directory string) ([]string, error)
	BootstrapSessions(ctx context.Context, executable, directory string) error
	ResumeClient(session Session, privateData string) (ClientInvocation, error)
}

type ClientInvocation struct {
	Arguments   []string
	Directory   string
	Environment map[string]string
}

func (r Registry) ClientLauncher(name string) (ClientLauncher, bool) {
	adapter, ok := r.adapters[name].(ClientLauncher)
	return adapter, ok
}
