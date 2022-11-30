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
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"go4.org/types"
	"golang.org/x/exp/slices"
	"tier.run/api"
	"tier.run/api/apitypes"
	"tier.run/api/materialize"
	"tier.run/client/tier"
	"tier.run/control"
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
		if isIsolationError(err) {
			log.Fatalf("tier: Running in isolated mode without the API key that started it; See 'tier switch -h'.")
		}
		if errors.Is(err, errUsage) {
			if err := help(stderr, cmd); err != nil {
				log.Fatalf("%v", err)
			}
			os.Exit(1)
		}
		log.Fatalf("tier: %v", err)
	}
}

func isIsolationError(err error) bool {
	var e *apitypes.Error
	return errors.As(err, &e) && e.Code == "account_invalid"
}

var (
	// only one trace per invoking (for now)
	traceID = newID()
)

func timeNow() types.Time3339 {
	return types.Time3339(time.Now()) // preserve time zone
}

func runTier(cmd string, args []string) (err error) {
	if f := background(); f != nil {
		defer f()
	} else {
		// background already processed
		return
	}

	if v := updateAvailable(); v != "" {
		fmt.Fprintf(stderr, "A new version of tier is available: %s\n", v)
		if isHomebrewInstall() {
			fmt.Fprintf(stderr, "Run `brew upgrade tier` to upgrade.\n")
		} else {
			fmt.Fprintf(stderr, "Visit https://tier.run/releases to download.\n")
		}
		fmt.Fprintln(stderr)
	}

	start := timeNow()
	p := loadProfile()
	defer func() {
		errStr := ""
		if errors.Is(err, errUsage) {
			errStr = "usage"
		} else if err != nil {
			errStr = err.Error()
		}

		trackEvent(&event{
			TraceID:     traceID,
			ID:          traceID,
			Type:        "cli",
			Name:        cmd,
			Start:       start,
			End:         timeNow(),
			Err:         errStr,
			AccountID:   p.AccountID,
			DisplayName: p.DisplayName,
			DeviceName:  p.DeviceName,
			Version:     version.String(),
			GOOS:        runtime.GOOS,
			GOARCH:      runtime.GOARCH,
		})
	}()

	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.Usage = func() {
		if err := help(stdout, cmd); err != nil {
			log.Fatalf("tier: %v", err)
		}
		os.Exit(2)
	}

	if slices.Contains(args, "-h") {
		err := help(stdout, cmd)
		return err
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

		err = pushJSON(ctx, f, func(f control.Feature, err error) {
			aid := cc().Stripe.AccountID
			if aid == "" && envAPIKey == "" {
				aid = p.AccountID
			}
			link, uerr := stripe.Link(cc().Live(), aid, "prices", f.ProviderID)
			if uerr != nil {
				panic(uerr)
			}
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
		})
		if errors.Is(err, control.ErrPlanExists) {
			//lint:ignore ST1005 this error is not used like normal errors
			return fmt.Errorf("illegal attempt to push features to existing plan(s); aborting.")
		}
		return err
	case "pull":
		data, err := tc().PullJSON(ctx)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "%s\n", data)
		return nil
	case "ls":
		m, err := tc().Pull(ctx)
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
		}, "\t"))

		for plan, p := range m.Plans {
			for feature, f := range p.Features {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\n",
					plan,
					feature,
					f.Mode,
					f.Aggregate,
					f.Base,
				)
			}
		}

		return nil
	case "connect":
		return connect()
	case "subscribe":
		fs := flag.NewFlagSet(cmd, flag.ExitOnError)
		email := fs.String("email", "", "sets the customer email address")
		if err := fs.Parse(args); err != nil {
			return err
		}
		if fs.NArg() == 0 {
			return errUsage
		}
		org := fs.Arg(0)
		p := &tier.ScheduleParams{
			Info: &tier.OrgInfo{
				Email: *email,
			},
		}
		var refs []string
		if fs.NArg() > 1 {
			refs = fs.Args()[1:]
			p.Phases = []tier.Phase{{Features: refs}}
		}
		vlogf("subscribing %s to %v", org, refs)
		return tc().Schedule(ctx, org, p)
	case "phases":
		if len(args) < 1 {
			return errUsage
		}
		org := args[0]
		p, err := tc().LookupPhase(ctx, org)
		if err != nil {
			return err
		}
		tw := newTabWriter()
		defer tw.Flush()
		fmt.Fprintln(tw, strings.Join([]string{
			"EFFECTIVE",
			"FEATURE",
			"PLAN",
		}, "\t"))
		for _, f := range p.Features {
			line := fmt.Sprintf("%s\t%s\t%s",
				p.Effective.Format(time.RFC3339),
				f.Name(),
				f.Plan(),
			)
			fmt.Fprintln(tw, line)
		}
		return nil
	case "limits":
		if len(args) < 1 {
			return errUsage
		}
		org := args[0]
		ur, err := tc().LookupLimits(ctx, org)
		if err != nil {
			return err
		}
		tw := newTabWriter()
		defer tw.Flush()
		fmt.Fprintln(tw, "FEATURE\tLIMIT\tUSED")
		for _, u := range ur.Usage {
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
		return tc().Report(ctx, org, feature, n)
	case "whoami":
		who, err := tc().WhoAmI(ctx)
		if err != nil {
			return err
		}
		tw := newTabWriter()
		defer tw.Flush()
		fmt.Fprintf(tw, "ID:\t%v\n", who.ProviderID)
		fmt.Fprintf(tw, "KeySource:\t%v\n", who.KeySource)
		fmt.Fprintf(tw, "Isolated:\t%v\n", who.Isolated)
		fmt.Fprintf(tw, "Email:\t%v\n", who.Email)
		fmt.Fprintf(tw, "Created:\t%v\n", who.Created.Format(time.RFC3339))
		fmt.Fprintf(tw, "URL:\t%v\n", who.URL)
		return nil
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
	case "switch":
		fs := flag.NewFlagSet("switch", flag.ExitOnError)
		create := fs.Bool("c", false, "create a new isolated environment")
		if err := fs.Parse(args); err != nil {
			return err
		}

		var a stripe.Account
		if *create {
			if cc().Live() {
				return fmt.Errorf("switch -c not allowed in live mode")
			}
			if fs.NArg() != 0 {
				return fmt.Errorf("switch does not accept arguments")
			}
			ca, _ := getState()
			if ca.ID != "" {
				//lint:ignore ST1005 we're using errors for text in main, ignore.
				return errors.New(`tier.state file present

To switch to an ioslated account, run from a different directory, or remove
the tier.state file.`)
			}
			var err error
			a, err = createAccount(ctx)
			if errors.Is(err, stripe.ErrConnectUnavailable) {
				fmt.Fprintf(stderr, "tier: stripe connect not enabled\n")
				return errUsage
			}
			if err != nil {
				return err
			}
		} else {
			if fs.NArg() < 1 {
				return errUsage
			}
			aid := fs.Arg(0)
			u, _ := url.Parse(aid)
			if u != nil {
				parts := strings.Split(u.Path, "/")
				for _, p := range parts {
					if strings.HasPrefix(p, "acct_") {
						aid = p
						break
					}
				}
			}
			if !strings.HasPrefix(aid, "acct_") {
				return fmt.Errorf("invalid account id or URL: %s", aid)
			}
			a.ID = aid
		}
		if err := saveState(a); err != nil {
			return err
		}
		fmt.Fprintf(stdout, strings.TrimSpace(`
Running in isolation mode.

To switch back to normal mode, you can either:

    A) delete the tier.state file in this directory, or
    B) run tier from another directory

The account dashboard is located at:

    https://dashboard.stripe.com/%s/test
`), a.ID)
		fmt.Fprintln(stdout)
		return nil
	case "clean":
		fs := flag.NewFlagSet("clean", flag.ExitOnError)
		accountAge := fs.Duration("switchaccounts", -1, "garbage collect switch accounts older than a duration; default is -1")
		if err := fs.Parse(args); err != nil {
			return err
		}
		if *accountAge >= 0 {
			return cleanAccounts(*accountAge)
		}
		return errUsage
	default:
		return errUsage
	}
}

func fileOrStdin(fname string) (io.ReadCloser, error) {
	if fname == "" {
		return nil, errUsage
	}
	if fname == "-" {
		return io.NopCloser(stdin), nil
	}
	return os.Open(fname)
}

var debugLevel, _ = strconv.Atoi(os.Getenv("TIER_DEBUG"))

func vlogf(format string, args ...any) {
	if *flagVerbose || debugLevel > 0 {
		// mimic behavior of log.Printf
		line := fmt.Sprintf("tierDEBUG: "+format, args...)
		if len(line) > 0 && line[len(line)-1] != '\n' {
			line = line + "\n"
		}
		io.WriteString(stderr, line)
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
	return cc().Push(ctx, fs, cb)
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

var tierClient *tier.Client

func tc() *tier.Client {
	h := api.NewHandler(cc(), vlogf)
	if tierClient == nil {
		// TODO(bmizerany): hookup logging, timeouts, etc
		tierClient = &tier.Client{
			HTTPClient: &http.Client{
				Transport: &clientTransport{h},
			},
		}
	}
	return tierClient
}

type clientTransport struct {
	h http.Handler
}

func (t *clientTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// While it feels a little odd to bring in httptest here, it's what I
	// want: The ability to run all commands through the API handler, and
	// get back a response, all without having to spawn a server listening
	// on a port. If I did spawn a server, it would add extra latency to
	// the cli which could easily be avoided. Still, it feels like a hack,
	// but it works great.
	w := httptest.NewRecorder()
	t.h.ServeHTTP(w, req)
	return w.Result(), nil
}

func loadProfile() *profile.Profile {
	p, err := profile.Load("tier")
	if err != nil {
		vlogf("tier: %v", err)
		p = &profile.Profile{
			DeviceName: "profile.unknown",
		}
	}
	return p
}
