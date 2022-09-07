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
	"text/tabwriter"

	"golang.org/x/exp/slices"
	"tier.run/cmd/tier/profile"
	"tier.run/pricing"
	"tier.run/pricing/schema"
	"tier.run/values"
)

// Flags
var (
	flagLive = flag.Bool("live", false, "use live Stripe key (default is false)")
)

// Env
var (
	envAPIKey = os.Getenv("STRIPE_API_KEY")
)

// resettable IO for testing
var (
	stdin  io.Reader = os.Stdin
	stdout io.Writer = os.Stdout
	stderr io.Writer = os.Stderr
)

// Errors
var (
	errUsage      = errors.New("usage: tier [--live] <connect|push|pull|ls> [<args>]")
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
			status := "ok"
			var reason string
			switch e.Err {
			case nil:
				reason = "created"
			case pricing.ErrFeatureExists:
				reason = "feature already exists"
			case pricing.ErrPlanExists:
				reason = "plan already exists"
			default:
				status = "failed"
				reason = e.Err.Error()
			}

			if e.Feature == "" && reason == "" {
				return // no need to report plan creation
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

		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		defer tw.Flush()

		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			"PLAN",
			"FEATURE",
			"MODE",
			"AGG",
			"BASE",
			"LINK",
		)

		for _, p := range m.Plans {
			link, err := url.JoinPath(dashURL[tc().Live()], "products", pricing.MakeID(p.ID))
			if err != nil {
				return err
			}

			for _, f := range p.Features {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\n",
					f.Plan,
					f.ID,
					values.Coalesce(f.Mode, "licensed"),
					values.Coalesce(f.Aggregate, "-"),
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
		key, err := getKey()
		if err != nil {
			fmt.Fprintf(stderr, "tier: There was an error looking up your Stripe API Key: %v\n", err)
			if errors.Is(err, profile.ErrProfileNotFound) {
				fmt.Fprintf(stderr, "tier: Please run `tier connect` to connect your Stripe account\n")
			}
			os.Exit(1)
		}
		tierClient = &pricing.Client{StripeKey: key}
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
