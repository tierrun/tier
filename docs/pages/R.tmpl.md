- [Install Tier](#install-tier)
- [Connect to Stripe](#connect-to-stripe)
  - [Verify](#verify)

## Install Tier

1. [Download Tier](https://tier.run/download)
2. 

## Connect to Stripe
```
; tier connect
To authenticate with Stripe, copy your API key from:

	https://dashboard.stripe.com/test/apikeys (test key)
	https://dashboard.stripe.com/apikeys      (live key)

Stripe API key: ****************************************

Success.

---------
  -----
    -

Welcome to Tier.

You're now ready to push a pricing model to Stripe using Tier. To learn more,
please visit:

	https://tier.run/docs/push
```

**Environment variables**

If set, Tier will honor the environment variables:

* `STRIPE_API_KEY` - the API key to use for the CLI
* `STRIPE_DEVICE_NAME` - the device name for the CLI, visible in the Dashboard.

Example:

```
; STRIPE_API_KEY=sk_test_123 tier whoami
```

### Verify

Verify tier is the right version, or is connected to stripe with the right
account using the `tier version` and `tier whoami` commands, respectively.

**Verify your CLI version**

The `tier version` command reports the current version of the CLI which can be use to verify you're on the version you expect to be.

**Verify the CLI's stripe connection**

```
; tier version
```

To verify your API keys are set correctly:

```
; tier whoami
```

The output should match the account you expect. If it does not, you may run
`tier connect` or reset the environment variables to use different keys.

* define features
* push features
* pull features (import)
* 


  * features are the building block of pricing models
  * a feature represent a thing a customer can consume, possibly for a price, and maybe up to some limit.
  * features are _not_ shared
  * features define limits, pricing, and the billing period the limits and prices apply to
  * features are arranged into plans (why?)

* samples of features
  * minimal feature == free + no limit
  * basic feature == base price, no tiers
  * tiered feature = no feature base, 1 tier with a limit
  * tiered (1 tier, $0/unit, base only) == same as no tiers with base


* feature schema reference





* Observe that no matter how hard you try, you can't override a plan that already exists.
  * Copy and push hello plan
  * Verify plan has correct:
    * monthly billing interval
    * price is $1.00/mo
    * is licensed
    * 
  * Try adding and pushing another feature
  * Try changing a plan that was already pushed
  * Try adding two of the same plan@version to the model and pushing
  * Prevent tampering with Tier plans using Stripe key scopes

* Confirm setup with example model
  * pull empty model
  * push hello feature
  * check hello feature in stripe
  * check hello feature in pull

* Make first pass 
* build a full model
* push the full model
* push with confidence (safe)
  * idempotent
  * changes to already pushed plan does not update stripe
  * add plan + push (see others do not update)
* name rules
  * plan
    * has `plan:` prefix
    * has `[\w\d_:\.]+` name
    * has `[\w\d\.]+` version
  * feature
    * has `feature:` prefix
    * has `[\w\d_:\.]+` name
* building a new model
* building a model from existing stripe prices and products


```
// intercom.io starter plan:
{
        "plans": {
                "plan:starter@0": {
                        "features": {
                                "feature:base": {
                                        "description": "Base Price",
                                        "base": 7500,
                                },
                                "feature:seats": {
                                        "aggregate": "perpetual",
                                        "tiers": [
                                                {"upto": 2},
                                                {"upto": 25, "price": 1900},
                                        ],
                                },
                                "feature:reach": {
                                        "aggregate": "max/1000",
                                        "tiers": [
                                                {"upto": 1},
                                                {"upto": 5, "price": 5000},
                                        ],
                                },
                        },
                },
        },
}
```


























```

```