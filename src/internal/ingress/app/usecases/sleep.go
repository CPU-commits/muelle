package usecases

import (
	"context"
	"time"
)

// sleep espera d o hasta que ctx se cancela; devuelve false si se canceló.
func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
