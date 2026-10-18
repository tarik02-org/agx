package cli

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"connectrpc.com/connect"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/cli/ui"
	"github.com/tarik02-org/agx/internal/platform"
)

// login waits for the daemon's login stream without a deadline of its own: the attempt's
// expiry bounds it. An interrupt closes the stream, which makes the daemon cancel the attempt.
func login(ctx context.Context, paths platform.Paths, id string, browser bool) error {
	c, err := newClient(paths)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, platform.Interrupts()...)
	defer signal.Stop(signals)
	interrupted := make(chan os.Signal, 1)
	go func() {
		select {
		case sig := <-signals:
			interrupted <- sig
			cancel()
		case <-ctx.Done():
		}
	}()
	stream, err := c.secrets.Login(ctx, request(&v1.LoginRequest{SecretId: id}, c.discovery.Token))
	if err == nil {
		defer func() { _ = stream.Close() }()
		var wait *waiting
		for stream.Receive() {
			switch event := stream.Msg().Event.(type) {
			case *v1.LoginResponse_Started:
				if err := printLoginURL(id, event.Started.AuthorizationUrl); err != nil {
					return err
				}
				if browser {
					openBrowser(ctx, event.Started.AuthorizationUrl)
				}
				wait = startWaiting("Waiting for login… (Ctrl-C to cancel)")
			case *v1.LoginResponse_Finished:
				wait.Stop()
				return loginOutcome(id, event.Finished)
			}
		}
		wait.Stop()
		err = stream.Err()
	}
	select {
	case sig := <-interrupted:
		ui.Diagnose(ui.Error, "login to %s cancelled", id)
		code := 130
		if number, ok := sig.(syscall.Signal); ok {
			code = 128 + int(number)
		}
		return &ExitError{Code: code}
	default:
	}
	switch {
	case err == nil:
		return fmt.Errorf("daemon ended the login to %s without an outcome", id)
	case connect.CodeOf(err) == connect.CodeUnavailable:
		return fmt.Errorf("daemon stopped during the login; run agx secret login %s again", id)
	}
	return err
}

func loginOutcome(id string, finished *v1.LoginFinished) error {
	switch finished.Outcome {
	case v1.LoginFinished_OUTCOME_SUCCEEDED:
		// stdout carries only the URL unless a person reads it.
		out := ui.Stderr
		if ui.Stdout.Terminal() {
			out = ui.Stdout
		}
		_, err := fmt.Fprintln(out, out.Status(ui.OK, "Logged in to "+id+"."))
		return err
	case v1.LoginFinished_OUTCOME_EXPIRED:
		return fmt.Errorf("login link for %s expired; run agx secret login %s again", id, id)
	case v1.LoginFinished_OUTCOME_FAILED:
		return fmt.Errorf("login to %s failed: %s", id, finished.Message)
	default:
		return fmt.Errorf("login to %s ended with an unknown outcome", id)
	}
}

// printLoginURL keeps stdout to the URL alone so it can be piped to a browser opener; the
// instructions go to stderr for a person at a terminal.
func printLoginURL(id, authorization string) error {
	if ui.Stderr.Terminal() {
		ui.Note("Open this URL in a browser on this machine to log in to " + id + ":")
	}
	_, err := fmt.Fprintln(ui.Stdout, ui.Stdout.Link(authorization))
	return err
}

// openBrowser hands the URL to the platform's opener as a single argument, without a shell:
// on Windows, url.dll does not interpret & and other metacharacters as cmd's start would.
// Failing to open it only warns, since the printed URL still works.
func openBrowser(ctx context.Context, target string) {
	u, err := url.Parse(target)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		ui.Diagnose(ui.Warn, "not opening a browser for a non-HTTP authorization URL")
		return
	}
	name, args := "xdg-open", []string{target}
	switch runtime.GOOS {
	case "darwin":
		name = "open"
	case "windows":
		name, args = "rundll32", []string{"url.dll,FileProtocolHandler", target}
	}
	// The browser must survive the login command and its Ctrl-C.
	cmd := exec.CommandContext(context.WithoutCancel(ctx), name, args...)
	platform.Isolate(cmd)
	if err := cmd.Start(); err != nil {
		ui.Diagnose(ui.Warn, "cannot open a browser: %v; open the URL yourself", err)
		return
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	// Openers usually exit at once; one still running has handed the URL over.
	select {
	case err := <-exited:
		if err != nil {
			ui.Diagnose(ui.Warn, "cannot open a browser: %s failed; open the URL yourself", name)
		}
	case <-time.After(2 * time.Second):
	}
}

// waiting animates a line on stderr while a person watches a styled terminal; unstyled
// terminals get the line once, and other streams nothing.
type waiting struct {
	stop chan struct{}
	done chan struct{}
	once sync.Once
}

func startWaiting(text string) *waiting {
	if !ui.Stderr.Terminal() {
		return nil
	}
	if !ui.Stderr.Styled() {
		ui.Note(text)
		return nil
	}
	w := &waiting{stop: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(w.done)
		frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for i := 0; ; i++ {
			_, _ = fmt.Fprintf(ui.Stderr, "\r%s %s", ui.Stderr.Badge(ui.Info, frames[i%len(frames)]), text)
			select {
			case <-w.stop:
				_, _ = fmt.Fprint(os.Stderr, "\r\x1b[2K")
				return
			case <-ticker.C:
			}
		}
	}()
	return w
}

func (w *waiting) Stop() {
	if w == nil {
		return
	}
	w.once.Do(func() { close(w.stop) })
	<-w.done
}
