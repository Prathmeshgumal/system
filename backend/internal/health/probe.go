package health

import (
	"context"
	"fmt"
	"net/http"
)

// Probe calls url and returns nil only if it answers 200.
//
// It powers the `api healthcheck` subcommand: our distroless image has no
// curl or shell, so Docker's healthcheck runs the API binary to check itself.
func Probe(ctx context.Context, url string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("call %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s returned %d", url, resp.StatusCode)
	}
	return nil
}
