package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	limits     list feature limits for an org
	report     report usage
	whois      get the Stripe customer ID for an org
	help       print this help message

The flags are:

	-l, -live  use live Stripe key (default is false)
	-v         verbose output
	-h         show this message
`)
)

var topics = map[string]string{
	"version": `Usage:	
	
	tier version
	
Print the version of the Tier CLI.
`,

	"push": `Usage:

	tier [--live] push <filename>

Tier push pushes the pricing JSON in the provided filename to Stripe. To learn
more about how this works, please visit: https://tier.run/docs/cli/push

If the --live flag is provided, your accounts live mode will be used.
`,

	"pull": `Usage:

	tier [--live] pull 

Tier pull pulls the pricing JSON from Stripe and writes it to stdout.

If the --live flag is provided, your accounts live mode will be used.
`,

	"connect": `Usage:

	tier connect

Tier connect creates a Stripe Connect account and writes them to
~/.config/tier/config.json for use with push, pull, and other commands that
interact with Stripe.
`,

	"ls": `Usage:

	tier [--live] ls

Tier ls lists all features in stripe.

If the --live flag is provided, your accounts live mode will be used.

The output is in the format:

	PLAN            FEATURE          MODE       AGG  BASE      LINK
	plan:free@1     feature:bar      graduated  sum  0         https://dashboard.stripe.com/test/prices/price_1LhjMLCdYGloJaDMWTEocuaj
	plan:free@1     feature:convert  graduated  sum  0         https://dashboard.stripe.com/test/prices/price_1LhjMLCdYGloJaDMmhWG3i5D
	plan:pro@0      feature:convert  graduated  sum  0         https://dashboard.stripe.com/test/prices/price_1LhjMLCdYGloJaDM5COLDSY1
`,
	"phases": `Usage:

	tier [--live] phases <org>

Tier phases lists all phases scheduled by Tier for the provided org.

If the --live flag is provided, your accounts live mode will be used.

The output is in the format:

	ORG        EFFECTIVE                  FEATURE                 PLAN
	org:blake  2022-10-10T23:26:10-07:00  feature:convert:temp    plan:pro@0
	org:blake  2022-10-10T23:26:10-07:00  feature:convert:volume  plan:pro@0
	org:blake  2022-10-10T23:26:10-07:00  feature:convert:weight  plan:pro@0
`,
	"subscribe": `Usage:

	tier [--live] subscribe <org> <phase>...

Tier subscribe creates or updates a subscription for the provided org, applying
the features in the plan.

If the --live flag is provided, your accounts live mode will be used.
`,
	"limits": `Usage:

	tier [--live] limits <org>

Tier limits lists the provided orgs limits and usage per feature subscribed to.

If the --live flag is provided, your accounts live mode will be used.
`,
	"report": `Usage:

	tier [--live] report <org> <feature> <n>

Tier report reports that n units of feature were used by org to Stripe.

For a report of usage, see the ("tier limits") command.

If the --live flag is provided, your accounts live mode will be used.
`,
	"whois": `Usage:

	tier [--live] whois <org>

Tier whois reports the Stripe customer ID for the provided org.

If the --live flag is provided, your accounts live mode will be used.
`,
	`tier`: errUsage.Error(),
}

func help(dst io.Writer, cmd string) error {
	switch cmd {
	case "help":
		// Prevent exiting with a non-zero stauts if help was
		// requested.
		io.WriteString(dst, errUsage.Error())
		return nil
	case "helpjson":
		data, err := json.MarshalIndent(topics, "", "\t")
		if err != nil {
			return err
		}
		dst.Write(data)
		return nil
	case "":
		return errUsage
	default:
		msg := topics[cmd]
		if msg == "" {
			return fmt.Errorf("tier: unknown help topic %q; Run 'tier help'", cmd)
		}
		io.WriteString(dst, msg)
		return nil
	}
}
