package stripe

import (
	"context"
	"errors"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
)

type Account struct {
	ID      string
	Created time.Time
	Type    string // default is "standard"
	Meta    Meta
}

type AccountParams struct {
	Type         string // default is "standard"
	Meta         Meta   // optional metadata to associate with the account
	BusinessName string // required for switch accounts
}

var (
	ErrConnectUnavailable = errors.New("connect unavailable")
)

// CreateAccount creates a new connected standard account.
func CreateAccount(ctx context.Context, c *Client, p *AccountParams) (Account, error) {
	if p == nil {
		p = &AccountParams{}
	}
	var f Form
	f.Set("business_profile", "name", p.BusinessName)
	if p.Type == "" {
		f.Set("type", "standard")
	} else {
		f.Set("type", p.Type)
	}
	for k, v := range p.Meta {
		f.Set("metadata", k, v)
	}
	var a jsonAccount
	err := c.Do(ctx, "POST", "/v1/accounts", f, &a)
	var e *Error
	if errors.As(err, &e) && strings.Contains(e.Message, "/docs/connect") {
		return Account{}, ErrConnectUnavailable
	}
	if err != nil {
		return Account{}, err
	}
	return a.Account(), nil
}

type jsonAccount struct {
	ID
	Type    string
	Meta    Meta `json:"metadata"`
	Created int
}

func (a jsonAccount) Account() Account {
	return Account{
		ID:      a.ProviderID(),
		Type:    a.Type,
		Meta:    a.Meta,
		Created: time.Unix(int64(a.Created), 0),
	}
}

func CleanAccounts(ctx context.Context, c *Client, f func(Account) bool) error {
	it := List[jsonAccount](ctx, c, "GET", "/v1/accounts", Form{})
	var g errgroup.Group
	g.SetLimit(100) // limit is 30 per second
	for it.Next() {
		a := it.Value().Account()
		if !f(a) {
			continue
		}
		g.Go(func() error {
			return c.Do(ctx, "DELETE", "/v1/accounts/"+a.ID, Form{}, nil)
		})
	}
	if err := it.Err(); err != nil {
		g.Wait() // wait for any in-flight requests
		return err
	}
	return g.Wait()
}
