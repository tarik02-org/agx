package cli

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"connectrpc.com/connect"

	"github.com/pkg/browser"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
	"github.com/tarik02-org/agx/internal/cli/ui"
	"github.com/tarik02-org/agx/internal/platform"
)

// login waits for the daemon's login stream without a deadline of its own: the attempt's
// expiry bounds it. An interrupt closes the stream, which makes the daemon cancel the attempt.
// In JSON mode each event of the stream is a document on stdout, and a browser opens only for a
// person reading stdout on a terminal.
func login(ctx context.Context, paths platform.Paths, id string, browser, autostart bool) error {
	if scripting.enabled && !ui.Stdout.Terminal() {
		browser = false
	}
	c, err := daemonClient(ctx, paths, autostart)
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
	stream, err := c.secrets.Login(ctx, connect.NewRequest(&v1.LoginRequest{SecretId: id}))
	if err == nil {
		defer func() { _ = stream.Close() }()
		var wait *waiting
		for stream.Receive() {
			switch event := stream.Msg().Event.(type) {
			case *v1.LoginResponse_Started:
				if ui.Stderr.Terminal() {
					ui.Note("Open this URL in a browser on this machine to log in to " + id + ":")
				}
				// stdout carries the URL alone, or its event, so it can be piped to a browser opener.
				err := emit(stream.Msg(), func() error {
					_, err := fmt.Fprintln(ui.Stdout, ui.Stdout.Link(event.Started.AuthorizationUrl))
					return err
				})
				if err != nil {
					return err
				}
				if browser {
					openBrowser(event.Started.AuthorizationUrl)
				}
				wait = startWaiting("Waiting for login… (Ctrl-C to cancel)")
			case *v1.LoginResponse_Finished:
				wait.Stop()
				if scripting.enabled {
					if err := printJSON(stream.Msg()); err != nil {
						return err
					}
					if event.Finished.Outcome != v1.LoginFinished_OUTCOME_SUCCEEDED {
						return &ExitError{Code: 1}
					}
					return nil
				}
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

func openBrowser(target string) {
	u, err := url.Parse(target)
	if err != nil || u.Scheme != "http" && u.Scheme != "https" {
		ui.Diagnose(ui.Warn, "not opening a browser for a non-HTTP authorization URL")
		return
	}
	browser.Stdout, browser.Stderr = io.Discard, io.Discard
	exited := make(chan error, 1)
	go func() { exited <- browser.OpenURL(target) }()
	select {
	case err := <-exited:
		if err != nil {
			ui.Diagnose(ui.Warn, "cannot open a browser: %v; open the URL yourself", err)
		}
	case <-time.After(2 * time.Second):
	}
}

// waiting animates a line on stderr while a person watches a styled terminal; unstyled
// terminals and JSON mode get the line once, and other streams nothing.
type waiting struct {
	stop chan struct{}
	done chan struct{}
	once sync.Once
}

func startWaiting(text string) *waiting {
	if !ui.Stderr.Terminal() {
		return nil
	}
	if !ui.Stderr.Styled() || scripting.enabled {
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
