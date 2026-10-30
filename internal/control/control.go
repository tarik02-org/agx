package control

import (
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/tarik02-org/agx/internal/platform"
)

// Version is set at build time with -ldflags "-X github.com/tarik02-org/agx/internal/control.Version=..."; otherwise it
// is the main module's version, which go install module@version records.
var Version = "dev"

func init() {
	if Version != "dev" {
		return
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		Version = strings.TrimPrefix(info.Main.Version, "v")
	}
}

// ErrUnavailable marks ReadDiscovery errors that leave no daemon to reach: it is not running,
// or its discovery file is invalid.
var ErrUnavailable = errors.New("daemon unavailable")

// unavailable keeps a discovery error's own message while matching ErrUnavailable.
type unavailable string

func (u unavailable) Error() string { return string(u) }

func (u unavailable) Is(target error) bool { return target == ErrUnavailable }

type Discovery struct {
	Version    string `json:"version"`
	InstanceID string `json:"instance_id"`
	URL        string `json:"url"`
	Token      string `json:"token"`
	Config     string `json:"config"`
	// PID is the daemon's process, for diagnosing one that stopped answering.
	PID int `json:"pid,omitempty"`
}

// ReadDiscovery reads the authenticated endpoint for this configuration.
func ReadDiscovery(paths platform.Paths) (Discovery, error) {
	var d Discovery
	path := filepath.Join(paths.Runtime, "daemon.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return d, unavailable("daemon unavailable; run agx daemon start")
	}
	if err := json.Unmarshal(b, &d); err != nil {
		return d, unavailable("invalid daemon discovery; restart the daemon")
	}
	u, err := url.Parse(d.URL)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.User != nil || u.Path != "" ||
		u.RawQuery != "" ||
		u.Fragment != "" ||
		d.Token == "" ||
		d.InstanceID == "" {
		return d, unavailable("invalid daemon discovery")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return d, unavailable("invalid daemon port")
	}
	return d, nil
}

const (
	ShutdownGrace  = 10 * time.Second
	StoppingHeader = "Agx-Daemon-Stopping"
	EndedHeader    = "Agx-Launch-Ended"
)
