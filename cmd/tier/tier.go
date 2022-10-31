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
	"golang.org/x/exp/slices"
	"tier.run/api/materialize"
	"tier.run/client/tier"
	"tier.run/control"
	"tier.run/profile"
	"tier.run/refs"
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

	if slices.Contains(args, "-h") {
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

		if err := pushJSON(ctx, f, func(f control.Feature, err error) {
			link := makeLink(f)
			var status, reason string
			switch err {
			case nil:
				status = "ok"
				reason = "created"
			case control.ErrFeatureExists:
				status = "ok"
				reason = "feature already exists"
			default:
				status = "failed"
				reason = err.Error()
				link = "-"
			}

			fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t[%s]\n",
				status,
				f.Plan(),
				f.Name(),
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

		tw := newTabWriter()
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
				f.Plan(),
				f.Name(),
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
		refs := args[1:]
		if org == "" || len(refs) == 0 {
			return errUsage
		}
		vlogf("subscribing %s to %v", org, refs)
		return tc().SubscribeToRefs(ctx, org, refs)
	case "phases":
		if len(args) < 1 {
			return errUsage
		}
		org := args[0]
		ps, err := tc().LookupPhases(ctx, org)
		if err != nil {
			return err
		}
		tw := newTabWriter()
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
					f.Name(),
					f.Plan(),
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
		use, err := tc().LookupLimits(ctx, org)
		if err != nil {
			return err
		}
		tw := newTabWriter()
		defer tw.Flush()
		fmt.Fprintln(tw, "FEATURE\tLIMIT\tUSED")
		for _, u := range use {
			limit := strconv.Itoa(u.Limit)
			if u.Limit == tier.Inf {
				limit = "∞"
			}
			fmt.Fprintf(tw, "%s\t%s\t%d\n",
				u.Feature,
				limit,
				u.Used,
			)
		}
		return nil
	case "report":
		org, feature, sn := getArg(args, 0), getArg(args, 1), getArg(args, 2)
		if org == "" || feature == "" || sn == "" {
			return errUsage
		}
		n, err := strconv.Atoi(sn)
		if err != nil {
			return err
		}

		fn, err := refs.ParseName(feature)
		if err != nil {
			return err
		}

		return tc().ReportUsage(ctx, org, fn, control.Report{
			At: time.Now(),
			N:  n,
			// TODO(bmizerany): suuport Clobber
		})
	case "whois":
		if len(args) < 1 {
			return errUsage
		}
		org := args[0]
		cid, err := tc().WhoIs(ctx, org)
		if errors.Is(err, control.ErrOrgNotFound) {
			return fmt.Errorf("no customer found for %q", org)
		}
		if err != nil {
			return err
		}
		fmt.Fprintln(stdout, cid)
		return nil
	case "serve":
		fs := flag.NewFlagSet("serve", flag.ExitOnError)
		addr := fs.String("addr", ":8080", "address to listen on (default ':8080')")
		if err := fs.Parse(args); err != nil {
			return err
		}
		return serve(*addr)
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

var tierClient *control.Client

func tc() *control.Client {
	if tierClient == nil {
		key, err := getKey()
		if err != nil {
			fmt.Fprintf(stderr, "tier: There was an error looking up your Stripe API Key: %v\n", err)
			if errors.Is(err, profile.ErrProfileNotFound) {
				fmt.Fprintf(stderr, "tier: Please run `tier connect` to connect your Stripe account\n")
			}
			os.Exit(1)
		}

		if stripe.IsLiveKey(key) {
			if !*flagLive {
				fmt.Fprintf(stderr, "tier: --live is required if stripe key is a live key\n")
				os.Exit(1)
			}
		} else {
			if *flagLive {
				fmt.Fprintf(stderr, "tier: --live provided with test key\n")
				os.Exit(1)
			}
		}

		sc := &stripe.Client{
			APIKey:    key,
			KeyPrefix: os.Getenv("TIER_KEY_PREFIX"),
			Logf:      vlogf,
		}
		tierClient = &control.Client{
			Stripe: sc,
			Logf:   vlogf,
		}
	}
	return tierClient
}

var debugLevel, _ = strconv.Atoi(os.Getenv("TIER_DEBUG"))

func vlogf(format string, args ...any) {
	if *flagVerbose || debugLevel > 0 {
		// mimic behavior of log.Printf
		line := fmt.Sprintf(format, args...)
		if len(line) > 0 && line[len(line)-1] != '\n' {
			line = line + "\n"
		}
		io.WriteString(stderr, line)
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

func pushJSON(ctx context.Context, r io.Reader, cb func(control.Feature, error)) error {
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

func makeLink(f control.Feature) string {
	link, err := url.JoinPath(dashURL[tc().Live()], "products", f.ID())
	if err != nil {
		panic(err)
	}
	return link
}

func newTabWriter() *tabwriter.Writer {
	return tabwriter.NewWriter(stdout, 0, 2, 2, ' ', 0)
}

func getArg(args []string, i int) string {
	if i < len(args) {
		return args[i]
	}
	return ""
}
