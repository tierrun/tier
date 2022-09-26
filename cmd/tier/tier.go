package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"text/tabwriter"
	"time"

	"go4.org/types"
	"golang.org/x/sync/errgroup"
	"tier.run/features"
	"tier.run/materialize"
	"tier.run/profile"
	"tier.run/stripe"
	"tier.run/version"
)

// Flags
var (
	flagLive    = flag.Bool("live", false, "use live Stripe key (default is false)")
	flagVerbose = flag.Bool("v", false, "verbose output")
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
	errUsage = errors.New("usage: tier [--live] <version|connect|push|pull|ls> [<args>]")
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

var (
	// only one trace per invoking (for now)
	traceID = newID()
)

func timeNow() types.Time3339 {
	return types.Time3339(time.Now()) // preserve time zone
}

func tier(cmd string, args []string) (err error) {
	start := timeNow()
	defer func() {
		p, err := profile.Load("tier")
		if err != nil {
			vlogf("tier: %v", err)
			p = &profile.Profile{}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		report.send(ctx, &event{
			TraceID:     traceID,
			ID:          traceID,
			Type:        "cli",
			Name:        cmd,
			Start:       start,
			End:         timeNow(),
			Err:         err,
			AccountID:   p.AccountID,
			DisplayName: p.DisplayName,
			DeviceName:  p.DeviceName,
			Version:     version.String(),
		})

		report.flush()
	}()

	ctx := context.Background()
	switch cmd {
	case "version":
		fmt.Println(version.String())
		return nil
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

		if err := pushJSON(ctx, f, func(f features.Feature, err error) {
			link, lerr := url.JoinPath(dashURL[tc().Live()], "products", f.ID())
			if lerr != nil {
				panic(lerr)
			}

			var status, reason string
			switch err {
			case nil:
				status = "ok"
				reason = "created"
			case features.ErrFeatureExists:
				status = "ok"
				reason = "feature already exists"
			default:
				status = "failed"
				reason = err.Error()
				link = "-"
			}

			fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t[%s]\n",
				status,
				f.Plan,
				f.Name,
				link,
				reason,
			)
		}); err != nil {
			return err
		}
		return nil
	case "pull":
		fs, err := tc().Pull(ctx, 0)
		if err != nil {
			return err
		}

		out, err := materialize.ToPricingJSON(fs)
		if err != nil {
			return err
		}

		fmt.Fprintf(stdout, "%s\n", out)

		return nil
	case "ls":
		fs, err := tc().Pull(ctx, 0)
		if err != nil {
			return err
		}

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

		for _, f := range fs {
			link, err := url.JoinPath(dashURL[tc().Live()], "products", f.ID())
			if err != nil {
				return err
			}

			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\n",
				f.Plan,
				f.Name,
				f.Mode,
				f.Aggregate,
				f.Base,
				link,
			)
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

var tierClient *features.Client

func tc() *features.Client {
	if tierClient == nil {
		key, err := getKey()
		if err != nil {
			fmt.Fprintf(stderr, "tier: There was an error looking up your Stripe API Key: %v\n", err)
			if errors.Is(err, profile.ErrProfileNotFound) {
				fmt.Fprintf(stderr, "tier: Please run `tier connect` to connect your Stripe account\n")
			}
			os.Exit(1)
		}
		sc := &stripe.Client{
			APIKey: key,
		}
		tierClient = &features.Client{
			Stripe: sc,
			Logf:   vlogf,
		}
	}
	return tierClient
}

func vlogf(format string, args ...interface{}) {
	if *flagVerbose {
		fmt.Fprintf(stderr, format, args...)
		fmt.Fprintln(stderr)
	}
}

// newID returns a random, hex encoded ID.
func newID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(buf[:])
}

func pushJSON(ctx context.Context, r io.Reader, cb func(features.Feature, error)) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}

	fs, err := materialize.FromPricingHuJSON(data)
	if err != nil {
		return err
	}
	var g errgroup.Group
	g.SetLimit(20)
	for _, f := range fs {
		f := f
		g.Go(func() error {
			err := tc().Push(ctx, f)
			cb(f, err)
			return nil
		})
	}
	return g.Wait()
}
