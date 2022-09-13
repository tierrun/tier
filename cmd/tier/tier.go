package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"text/tabwriter"
	"time"

	"tier.run/cmd/tier/profile"
	"tier.run/pricing"
	"tier.run/pricing/schema"
	"tier.run/values"
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

func tier(cmd string, args []string) (err error) {
	start := time.Now()
	defer func() {
		report(event{
			Type:  "cli",
			Name:  cmd,
			Start: start,
			End:   time.Now(),
			Err:   err,
		})
		flushEvents()
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
				values.Coalesce(e.Feature, "-"),
				link,
				reason,
			)
		}); err != nil {
			return err
		}
		return nil
	case "pull":
		m, err := tc().Pull(ctx)
		if err != nil {
			return err
		}

		m.Plans = filterNonTierPlans(m.Plans)

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

		m.Plans = filterNonTierPlans(m.Plans)

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
			for _, f := range p.Features {
				link, err := url.JoinPath(dashURL[tc().Live()], "prices", f.ProviderID)
				if err != nil {
					return err
				}

				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%s\n",
					f.Plan,
					values.Coalesce(f.ID, "-"),
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
	var dst schema.Plans
	for _, p := range plans {
		if p.ID != "" {
			dst = append(dst, p)
		}
	}
	return dst
}

type event struct {
	TraceID string
	ID      string

	Type  string
	Name  string
	Start time.Time
	End   time.Time
	Err   error

	AccountID   string
	DisplayName string
	DeviceName  string
	Version     string
}

func (e *event) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		AccountID string
		Type      string
		Start     string
		End       string
		Err       string
	}{
		AccountID: e.AccountID,
		Type:      e.Type,
		Start:     e.Start.Format(time.RFC3339),
		End:       e.End.Format(time.RFC3339),
		Err:       e.Err.Error(),
	})
}

var events []event

func report(ev event) {
	events = append(events, ev)
}

func flushEvents() {
	f := func() error {
		p, err := profile.Load("tier")
		if err != nil {
			report(event{
				Type:  "cli",
				Name:  "flush/profile.Load",
				Start: time.Now(),
				End:   time.Now(),
				Err:   err,
			})
		}

		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		for _, ev := range events {
			ev.TraceID = traceID
			ev.ID = traceID

			if p != nil {
				ev.AccountID = p.AccountID
				ev.DisplayName = p.DisplayName
				ev.DeviceName = p.DeviceName
			}

			ev.Version = version.String()

			if err := enc.Encode(ev); err != nil {
				return err
			}
		}

		req, err := http.NewRequest(http.MethodPost, "https://tele.tier.run/api/t", &buf)
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "tier-cli") // TODO: include version, commit, etc

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		resp.Body.Close() // fire-and-forget
		return nil
	}
	if err := f(); err != nil {
		vlogf("failed to report event: %v", err)
	}
}

func vlogf(format string, args ...interface{}) {
	if *flagVerbose {
		fmt.Fprintf(stderr, format, args...)
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
