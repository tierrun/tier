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
	"tier.run/api/materialize"
	"tier.run/client/tier"
	"tier.run/control"
	"tier.run/profile"
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
	if sendEvents() {
		return
	}

	v, err := checkForUpdate()
	if err != nil {
		vlogf("%v", err)
		// do not exit, continue
	}
	if v != "" {
		fmt.Fprintf(stderr, "A new version of tier is available: %s\n", v)
		if isHomebrewInstall() {
			fmt.Fprintf(stderr, "Run `brew upgrade tier` to upgrade.\n")
		} else {
			fmt.Fprintf(stderr, "Visit https://tier.run/releases to download.\n")
		}
		fmt.Fprintln(stderr)
	}

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
			os.Exit(1)
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
	defer flushEvents()

	start := timeNow()
	defer func() {
		p, pErr := profile.Load("tier")
		if pErr != nil {
			vlogf("tier: %v", err)
			p = &profile.Profile{
				DeviceName: "profile.unknown",
			}
		}

		errStr := ""
		if err != nil && !errors.Is(err, errUsage) {
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

		return pushJSON(ctx, f, func(f control.Feature, err error) {
			link, uerr := url.JoinPath(dashURL[cc().Live()], "products", f.ProviderID)
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
		if len(args) < 2 {
			return errUsage
		}
		org := args[0]
		refs := args[1:]
		if org == "" || len(refs) == 0 {
			return errUsage
		}
		vlogf("subscribing %s to %v", org, refs)
		return tc().Subscribe(ctx, org, refs...)
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
		line := fmt.Sprintf("tier: "+format, args...)
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
