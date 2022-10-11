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
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"go4.org/types"
	"tier.run/client/tier"
	"tier.run/materialize"
	"tier.run/profile"
	"tier.run/stripe"
	"tier.run/version"
)

// Flags
var (
	flagLive     = flag.Bool("live", false, "use live Stripe key (default is false)")
	flagVerbose  = flag.Bool("v", false, "verbose output")
	flagMainHelp = flag.Bool("h", false, "show this message")
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
	// TODO(bmizerany): generate subcommand lists from help map
	//lint:ignore ST1005 this error is not used like normal errors
	errUsage = errors.New(`Usage:

	tier [flags] <command> [arguments]

The commands are:

	connect    connect your Stripe account
	push       push pricing plans to Stripe
	pull       pull pricing plans from Stripe
	ls         list pricing plans
	version    print the current CLI version
	subscribe  subscribe an org to a pricing plan
	phases     list scheduled phases for an org

The flags are:

	-l, -live  use live Stripe key (default is false)
	-v         verbose output
	-h         show this message
`)
)

func main() {
	log.SetFlags(0)
	flag.Usage = func() {
		if err := help(stderr, ""); err != nil {
			log.Fatalf("%v", err)
		}
	}
	flag.Parse()
	if *flagMainHelp {
		flag.Usage()
	}
	args := flag.Args()
	if len(args) == 0 {
		log.Fatalf("%v", errUsage)
	}

	cmd := args[0]
	if err := runTier(cmd, args[1:]); err != nil {
		if errors.Is(err, errUsage) {
			if err := help(stderr, cmd); err != nil {
				log.Fatalf("%v", err)
			}
			return
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

func runTier(cmd string, args []string) (err error) {
	start := timeNow()
	defer func() {
		p, err := profile.Load("tier")
		if err != nil {
			vlogf("tier: %v", err)
			p = &profile.Profile{
				DeviceName: "profile.unknown",
			}
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

	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.Usage = func() {
		if err := help(stdout, cmd); err != nil {
			log.Fatalf("tier: %v", err)
		}
		os.Exit(2)
	}
	flagHelp := fs.Bool("h", false, "help")
	fs.Parse(args)
	if *flagHelp {
		return help(stdout, cmd)
	}

	ctx := context.Background()
	switch cmd {
	case "help":
		if fs.NArg() == 0 {
			return errUsage
		}
		return help(stdout, args[0])
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

		if err := pushJSON(ctx, f, func(f tier.Feature, err error) {
			link := makeLink(f)
			var status, reason string
			switch err {
			case nil:
				status = "ok"
				reason = "created"
			case tier.ErrFeatureExists:
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

		fmt.Fprintln(tw, strings.Join([]string{
			"PLAN",
			"FEATURE",
			"MODE",
			"AGG",
			"BASE",
			"LINK",
		}, "\t"))

		for _, f := range fs {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\n",
				f.Plan,
				f.Name,
				f.Mode,
				f.Aggregate,
				f.Base,
				makeLink(f),
			)
		}

		return nil
	case "connect":
		return connect()
	case "subscribe":
		if len(args) < 2 {
			return errUsage
		}
		org := args[0]
		plans := args[1:]
		if org == "" || len(plans) == 0 {
			return errUsage
		}
		plan := plans[0] // TODO(bmizerany): support multiple plans and feature folding

		vlogf("subscribing %s to %v", org, plan)
		return tc().SubscribeToPlan(ctx, org, plan)
	case "phases":
		if len(args) < 1 {
			return errUsage
		}
		org := args[0]
		ps, err := tc().LookupPhases(ctx, org)
		if err != nil {
			return err
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		defer tw.Flush()
		fmt.Fprintln(tw, strings.Join([]string{
			"ORG",
			"INDEX",
			"ACTIVE",
			"EFFECTIVE",
			"FEATURE",
			"PLAN",
		}, "\t"))
		for i, p := range ps {
			if i > 0 {
				fmt.Fprintln(tw)
			}
			active := "n"
			if p.Current {
				active = "Y"
			}
			for _, f := range p.Features {
				line := fmt.Sprintf("%s\t%d\t%s\t%s\t%s\t%s",
					p.Org,
					i,
					active,
					p.Effective.Format(time.RFC3339),
					f.Name,
					f.Plan,
				)
				fmt.Fprintln(tw, line)
			}
		}
		return nil
	case "limits":
		if len(args) < 1 {
			return errUsage
		}
		org := args[0]
		limits, err := tc().LookupLimits(ctx, org)
		if err != nil {
			return err
		}
		tw := tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
		defer tw.Flush()
		fmt.Fprintln(tw, strings.Join([]string{
			"FEATURE",
			"LIMIT",
		}, "\t"))
		for _, l := range limits {
			limit := strconv.Itoa(l.Limit)
			if l.Limit == tier.Inf {
				limit = "∞"
			}
			fmt.Fprintf(tw, "%s\t%s\n",
				l.Name,
				limit,
			)
		}
		return nil
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

var tierClient *tier.Client

func tc() *tier.Client {
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
			APIKey:    key,
			KeyPrefix: os.Getenv("TIER_KEY_PREFIX"),
		}
		tierClient = &tier.Client{
			Stripe: sc,
			Logf:   vlogf,
		}
	}
	return tierClient
}

var debugLevel, _ = strconv.Atoi(os.Getenv("TIER_DEBUG"))

func vlogf(format string, args ...any) {
	if *flagVerbose || debugLevel > 0 {
		fmt.Fprintf(stderr, format, args...)
		fmt.Fprintln(stderr)
	}
}

func vvlogf(format string, args ...any) {
	if debugLevel > 2 {
		vlogf(format, args...)
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

func pushJSON(ctx context.Context, r io.Reader, cb func(tier.Feature, error)) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	fs, err := materialize.FromPricingHuJSON(data)
	if err != nil {
		return err
	}
	tc().Push(ctx, fs, cb)
	return nil
}

func makeLink(f tier.Feature) string {
	link, err := url.JoinPath(dashURL[tc().Live()], "products", f.ID())
	if err != nil {
		panic(err)
	}
	return link
}
