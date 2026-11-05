package opencode

import (
	"slices"
	"strings"
	"testing"

	"github.com/tarik02-org/agx/internal/adapters"
)

func TestResumeUsesValidatedServiceRoute(t *testing.T) {
	session := adapters.Session{ID: "existing", Directory: "/native/server/location"}
	invocation, err := (Adapter{}).ResumeClient(
		session,
		`{"sessionId":"existing","directory":"/native/server/location","server":"http://127.0.0.1:1234","username":"opencode","password":"private-native-password"}`,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(
		invocation.Arguments,
		[]string{"--session", "existing", "--server", "http://127.0.0.1:1234", "/native/server/location"},
	) {
		t.Fatalf("native route lost: %v", invocation.Arguments)
	}
	if invocation.Directory != "" {
		t.Fatal("server location was treated as a local working directory")
	}
	if invocation.Environment["OPENCODE_PASSWORD"] != "private-native-password" ||
		strings.Contains(strings.Join(invocation.Arguments, " "), "private-native-password") {
		t.Fatal("native credential must reach only the private process environment")
	}
}

func TestResumeRejectsUnvalidatedIDAndEmbeddedCredential(t *testing.T) {
	session := adapters.Session{ID: "existing", Directory: "/remote"}
	for _, data := range []string{
		`{"sessionId":"other","directory":"/remote","server":"http://127.0.0.1:1234"}`,
		`{"sessionId":"existing","directory":"/remote","server":"http://user:password@127.0.0.1:1234"}`,
		`{"sessionId":"existing","directory":"/remote","server":"http://127.0.0.1:1234?token=private"}`,
		`{"sessionId":"existing","directory":"/remote","server":null}`,
	} {
		if _, err := (Adapter{}).ResumeClient(session, data); err == nil {
			t.Fatal("unvalidated native resume route accepted")
		}
	}
}
