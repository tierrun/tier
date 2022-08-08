package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
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

		return tc().PushJSON(ctx, f, func(plan, feature string, err error) {
			if feature == "" {
				return // no need to report plan creation
			}
			status := "created"
			if errors.Is(err, pricing.ErrFeatureExists) {
				status = "exists"
			}
			fmt.Printf("%s %s %s\n", plan, feature, status)
		})
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
		if err != nil {
			log.Fatalf("tier: %v", err)
		}
		// c.Logf = log.Printf // TODO: check for -v flag
		tierClient = c
	}
	return tierClient
}
