package stripe

import (
	"context"
	"errors"
	"time"

	"kr.dev/errorfmt"
	"tailscale.com/logtail/backoff"
)

func Dedup(ctx context.Context, key string, logf func(string, ...any), f func(f Form) error) (winner bool, err error) {
	defer errorfmt.Handlef("stripe: Dedup(%s): %w", key, &err)

	bo := backoff.NewBackoff("stripe", logf, 5*time.Second)
	for {
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		default:
		}

		var data Form
		data.SetIdempotencyKey(key)
		err = f(data)
		var e *Error
		if errors.As(err, &e) {
			switch {
			case e.Code == "idempotency_key_in_use":
				// retry
				bo.BackOff(ctx, err)
				continue
			case e.Type == "idempotency_error":
				return false, nil
			}
		}
		if err != nil {
			return false, err
		}
		return true, nil
	}
}
