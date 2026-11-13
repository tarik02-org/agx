package daemon

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	v1 "github.com/tarik02-org/agx/gen/agx/v1"
)

func TestProceduresNeedTheManagementToken(t *testing.T) {
	mux := http.NewServeMux()
	(&Server{drain: newDrain()}).handle(mux, "token")
	server := httptest.NewServer(mux)
	defer server.Close()
	services := v1.File_agx_v1_agx_proto.Services()
	for i := range services.Len() {
		methods := services.Get(i).Methods()
		for j := range methods.Len() {
			method := methods.Get(j)
			procedure := "/" + string(services.Get(i).FullName()) + "/" + string(method.Name())
			if selfAuthorized[procedure] {
				continue
			}
			contentType, body := "application/json", []byte("{}")
			if method.IsStreamingServer() || method.IsStreamingClient() {
				contentType, body = "application/connect+json", []byte{0, 0, 0, 0, 2, '{', '}'}
			}
			req, err := http.NewRequestWithContext(
				t.Context(),
				http.MethodPost,
				server.URL+procedure,
				bytes.NewReader(body),
			)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Content-Type", contentType)
			response, err := server.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			answer, err := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if err != nil || !strings.Contains(string(answer), `"unauthenticated"`) {
				t.Errorf("%s without a token answered %d %s (%v)", procedure, response.StatusCode, answer, err)
			}
		}
	}
}
