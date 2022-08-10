package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"

	"github.com/tierrun/tierx/pricing"
	"github.com/tierrun/tierx/pricing/schema"
	"golang.org/x/exp/slices"
)

var (
	stdin  io.Reader = os.Stdin
	stdout io.Writer = os.Stdout
	stderr io.Writer = os.Stderr
)

var (
	errUsage      = errors.New("usage: tier <connect|push|connect> [<args>]")
	errPushFailed = errors.New("push failed")
)

func main() {
	log.SetFlags(0)
	flag.Parse()
	args := flag.Args()
	if len(args) == 0 {
		log.Fatalf("%v", errUsage)
	}
	if err := tier(args[0], args[1:]); err != nil {
		if errors.Is(err, errUsage) {
			log.Fatalf("%v", err)
		} else {
			log.Fatalf("tier: %v", err)
		}
	}
}

var dashURL = map[bool]string{
	true:  "https://dashboard.stripe.com",
	false: "https://dashboard.stripe.com/test",
}

func tier(cmd string, args []string) error {
	ctx := context.Background()
	switch cmd {
	case "init":
		panic("TODO")
	case "push":
		pj := ""
		if len(args) > 0 {
			pj = args[0]
		}

		f, err := fileOrStdin(pj)
		if err != nil {
			return err
		}
		defer f.Close()

		if err := tc().PushJSON(ctx, f, func(e *pricing.PushEvent) {
			fmt.Fprintf(stdout, "%v\n", e)
			if e.Feature == "" {
				return // no need to report plan creation
			}
			status := "ok"

			reason := "created"
			if e.Err != nil {
				status = "failed"
				reason = e.Err.Error()
			}
			if errors.Is(e.Err, pricing.ErrFeatureExists) {
				reason = "already exists"
			}

			link, err := url.JoinPath(dashURL[tc().Live()], "products", e.PlanProviderID)
			if err != nil {
				reason = fmt.Sprintf("failed to create link: %v", err)
			}
			fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t[%s]\n",
				status,
				e.Plan,
				e.Feature,
				link,
				reason,
			)
		}); err != nil {
			if errors.As(err, &pricing.DecodeError{}) {
				return err
			}
			return errPushFailed
		}
		return nil
	case "pull":
		m, err := tc().Pull(ctx)
		if err != nil {
			return err
		}

		filterNonTierPlans(m.Plans)

		out, err := json.MarshalIndent(m, "", "     ")
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "%s\n", out)

		return nil
	case "ls":
		m, err := tc().Pull(ctx)
		if err != nil {
			return err
		}

		filterNonTierPlans(m.Plans)

		for _, p := range m.Plans {
			link, err := url.JoinPath(dashURL[tc().Live()], "products", pricing.MakeID(p.ID))
			if err != nil {
				return err
			}
			for _, f := range p.Features {
				fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%d\t%s\n",
					f.Plan,
					f.ID,
					f.Mode,
					f.Aggregate,
					f.Base,
					link,
				)
			}
		}

		return nil
	case "connect":
		return connect()
	default:
		return errUsage
	}
}

func fileOrStdin(fname string) (io.ReadCloser, error) {
	if fname == "" || fname == "-" {
		return io.NopCloser(stdin), nil
	}
	return os.Open(fname)
}

var tierClient *pricing.Client

func tc() *pricing.Client {
	if tierClient == nil {
		c, err := pricing.FromEnv()
		if err == nil {
			tierClient = c
			return tierClient
		}
		if errors.Is(err, pricing.ErrKeyNotSet) {
			key, err := getKey()
			if err != nil {
				fmt.Fprintf(stderr, "tier: There was an error looking up your Stripe API Key: %v\n", err)
				fmt.Fprintf(stderr, "tier: Please run `tier connect` to connect your Stripe account\n")
				os.Exit(1)
			}
			return &pricing.Client{StripeKey: key}
		}
		if err != nil {
			log.Fatalf("tier: %v", err)
		}
	}
	return tierClient
}

func filterNonTierPlans(plans schema.Plans) schema.Plans {
	for i, p := range plans {
		if p.ID == "" {
			plans = slices.Delete(plans, i, i+1)
		}
		for j, f := range p.Features {
			if f.Err != nil {
				p.Features = slices.Delete(p.Features, j, j+1)
			}
		}
	}
	return plans
}
