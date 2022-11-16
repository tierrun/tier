package stripe

import (
	"context"
	"errors"
	"strings"
)

type Account struct {
	ID string
}

var (
	ErrConnectUnavailable = errors.New("connect unavailable")
)

// CreateAccount creates a new connected standard account.
func CreateAccount(ctx context.Context, c *Client) (Account, error) {
	var f Form
	f.Set("type", "standard")
	var a Account
	err := c.Do(ctx, "POST", "/v1/accounts", f, &a)
	var e *Error
	if errors.As(err, &e) && strings.Contains(e.Message, "/docs/connect") {
		return Account{}, ErrConnectUnavailable
	}
	if err != nil {
		return Account{}, err
	}
	return a, nil
}
