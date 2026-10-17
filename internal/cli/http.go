package cli

import (
	"context"
	"net/http"

	"github.com/tarik02/weft/internal/securefs"
)

func newStopRequest(ctx context.Context, url, token string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return req, nil
}

func secureRequestID() string { return securefs.Token() }
