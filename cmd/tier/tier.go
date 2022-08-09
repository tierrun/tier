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
)

var errUsage = errors.New("usage: tier <push|pull> [<args>]")

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
			fmt.Fprintf(os.Stdout, "%s\t%s\t%s\t%s\t[%s]\n",
				status,
				e.Plan,
				e.Feature,
				link,
				reason,
			)
		}); err != nil {
			return errors.New("pushed failed")
		}
		return nil
	case "pull":
		m, err := tc().Pull(ctx)
		if err != nil {
			return err
		}

		out, err := json.MarshalIndent(m, "", "     ")
		if err != nil {
			return err
		}
		fmt.Printf("%s\n", out)

		return nil
	case "connect":
		return connect()
	default:
		return errUsage
	}
}

func fileOrStdin(fname string) (io.ReadCloser, error) {
	if fname == "" || fname == "-" {
		return io.NopCloser(os.Stdin), nil
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
				fmt.Fprintf(os.Stderr, "tier: There was an error looking up your Stripe API Key: %v\n", err)
				fmt.Fprintf(os.Stderr, "tier: Please run `tier connect` to connect your Stripe account\n")
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
