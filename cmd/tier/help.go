package main

import (
	"fmt"
	"io"
)

var topics = map[string]string{
	"version": `Usage:	
	
	tier version
	
Print the version of the Tier CLI.
`,

	"push": `Usage:

	tier push [--live] <filename>

Tier push pushes the pricing JSON in the provided filename to Stripe. To learn
more about how this works, please visit: https://tier.run/docs/cli/push

If the --live flag is provided, your accounts live mode will be used.
`,

	"pull": `Usage:

	tier pull [--live]

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

	tier ls [--live]

Tier ls lists all features in stripe.

If the --live flag is provided, your accounts live mode will be used.

The output is in the format:

	PLAN            FEATURE          MODE       AGG  BASE      LINK
	plan:free@1     feature:bar      graduated  sum  0         https://dashboard.stripe.com/test/prices/price_1LhjMLCdYGloJaDMWTEocuaj
	plan:free@1     feature:convert  graduated  sum  0         https://dashboard.stripe.com/test/prices/price_1LhjMLCdYGloJaDMmhWG3i5D
	plan:pro@0      feature:convert  graduated  sum  0         https://dashboard.stripe.com/test/prices/price_1LhjMLCdYGloJaDM5COLDSY1
`,
}

func help(dst io.Writer, cmd string) error {
	if cmd == "" {
		return errUsage
	}
	msg := topics[cmd]
	if msg == "" {
		return fmt.Errorf("tier: unknown help topic %q; Run 'tier help'", cmd)
	}
	io.WriteString(dst, msg)
	return nil
}
