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
	version    display the current CLI version
	subscribe  subscribe an org to a pricing plan
	phases     list scheduled phases for an org
	limits     list feature limits for an org
	report     report usage for metered features
	whoami     display the current account information
	switch     create and switch to clean rooms
	whois      display the Stripe customer ID for an org
	serve      run the sidecar API
	clean      remove objects in Stripe Test Mode
	help       display this help message

The flags are:

	-live      use live Stripe key (default is false)
	-v         verbose output
	-h         show this message

Environment variables:

	STRIPE_API_KEY

	  Stripe API key. If not set, the CLI will use
	  $HOME/.config/tier/config.json if present; otherwise an error will
	  occur.
`)
)

var topics = map[string]string{
	"version": `Usage:	
	
	tier version
	
Print the version of the Tier CLI.
`,

	"push": `Usage:

	tier [--live] push <filename | url | - >

"tier push" pushes pricing JSON to Stripe. The data may come from a file, url,
or stdin. If a URL is specified, push will use the response body from a GET
request as pricing JSON, if the status code is of the 2XX variety and the body
is valid pricing JSON. If the filename is ("-") then the pricing JSON is read
from stdin.

To learn more about how this works, please visit: https://tier.run/docs/cli/push

If the --live flag is provided, your accounts live mode will be used.
`,

	"pull": `Usage:

	tier [--live] pull 

Tier pull pulls the pricing JSON from Stripe and writes it to stdout.

If the --live flag is provided, your accounts live mode will be used.
`,

	"connect": `Usage:

	tier connect

Tier connect creates a set of Stripe restricted keys and writes them to
~/.config/tier/config.json for use with push, pull, and other commands that
interact with Stripe.

In order to use the generated restricted key for live mode pushes, it must
have additional permissions granted on the Stripe dashboard.

See https://www.tier.run/docs/cli/connect for more information.
`,

	"ls": `Usage:

	tier [--live] ls

Tier ls lists all features in stripe.

If the --live flag is provided, your accounts live mode will be used.

The output is in the format:

	PLAN            FEATURE          MODE       AGG  BASE
	plan:free@1     feature:bar      graduated  sum  0
	plan:free@1     feature:convert  graduated  sum  0
	plan:pro@0      feature:convert  graduated  sum  0
`,
	"phases": `Usage:

	tier [--live] phases <org>

Tier phases lists all phases scheduled by Tier for the provided org.

If the --live flag is provided, your accounts live mode will be used.

The output is in the format:

	EFFECTIVE                  FEATURE                 PLAN
	2022-10-10T23:26:10-07:00  feature:convert:temp    plan:pro@0
	2022-10-10T23:26:10-07:00  feature:convert:volume  plan:pro@0
	2022-10-10T23:26:10-07:00  feature:convert:weight  plan:pro@0
`,
	"subscribe": `Usage:

	tier [--live] subscribe [flags] <org> [plan|featurePlan]...

Tier subscribe creates or updates a subscription for the provided org, applying
the features in the plan.

Flags:

	--email
		set the org's email address
	--trial days
		set the org's trial period to the provided number of days. If
		negative, the tial period will last indefinitely, and no other
		phase will come after it.
	--cancel
		cancel the org's subscription. It is an error to provide a plan
		or featurePlan with this flag.
	--checkout=<success_url>
		subscribe the org to plans and features using Stripe Checkout.
		The success url is required, and may be used with the
		--cancel_url flag.
	--cancel_url=<cancel_url>
		specify a cancel_url for Stripe Checkout. This flag is ignored
		if --checkout is not set.

Global Flags:

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
	"whoami": `Usage:

	tier whoami

Tier whoami reports Stripe account information associated with the current key
in use as a result of "tier connect", "tier switch", or the STRIPE_API_KEY.
`,

	`tier`: errUsage.Error(),

	`serve`: `Usage:

	tier serve [--addr <addr>]

Tier serve starts a web server that exposes the Tier API over HTTP listening on
the provided service address.

The default service address is "localhost:8080".
`,
	"switch": `Usage:

	tier switch [flags] [accountID]

Tier switch tells tier to use the provided accountID, or, if run with "-c", to
create and use a new isolation account, when run from the current working
directory.

To switch back to the default account:

    a) rename the tier.state file
    b) change your working directory, or
    c) delete the tier.state file 

Example:

	This example demostrates pushing a pricing model to an isolated account
	and then starting fresh with a new isolated account, and then switching
	back to the default account by deleting the tier.state file.

		; tier connect
		; tier switch -c
		; tier push pricing.json
		; tier pull
		; rm tier.state
		; tier switch -c
		; tier pull
		; tier push pricing2.json
		; rm tier.state
		; tier pull

Prerequisites:

	Connected accounts MUST be enabled in your Stripe account. To enable,
	head to https://dashboard.stripe.com/test/connect/accounts/overview.

Constraints:

	Commands run in isloated mode for an account not accessible by the
	current API key, will fail. Move to a new directory or move the
	tier.state file to resume using the API key, or run "tier connect" and
	login to the root account that owns the account in the tier.state file.

Flags:

    -c
	Create a new account and switch to it.
`,
	"clean": `Usage:

	tier clean <flags>

Clean removes objects from Stripe Test Mode accounts. It can be used with a
cron job to keep your test accounts clean.

The -switchaccounts flag, takes a duration as its value and causes clean to
remove connected Stripe accounts created by the switch command. Accounts are
only considered for removal if they were created by the switch command, older
than the provided duration of time specified in the flag, and exist in Test
Mode. The duration may be an integer representing seconds, or a duration string
such as "1h30m". The default duration is -1 which disables the cleaning of
accounts.

Examples:

	; tier clean -switchaccounts 1h   # remove all switch accounts older than 1 hour
	; tier clean -switchaccounts 730h # remove all switch accounts older than 30 days
	; tier clean -switchaccounts 0    # remove all switch accounts
	; tier clean -switchaccounts -1   # nop
`,
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
