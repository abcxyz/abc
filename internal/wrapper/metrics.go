package wrapper

import (
	"context"

	"github.com/abcxyz/abc-updater/pkg/metrics"
	"github.com/abcxyz/pkg/logging"
)

// WriteMetric is an async wrapper for metrics.WriteMetric.
// It returns a function that blocks on completion and handles any errors.
func WriteMetric(ctx context.Context, client *metrics.Client, name string, count int64) func() {
	errCh := make(chan error, 1)
	go func() {
		defer close(errCh)
		errCh <- client.WriteMetric(ctx, name, count)
	}()

	return func() {
		err := <-errCh
		if err != nil {
			logger := logging.FromContext(ctx)
			logger.DebugContext(ctx, "Metric writing failed.", "err", err)
		}
	}
}
