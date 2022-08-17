
# Pricing Models

- [Pricing Models](#pricing-models)
  - [Introduction](#introduction)
  - [Pricing syntax](#pricing-syntax)
    - [Plans](#plans)
  - [Pricing samples](#pricing-samples)
  - [Scheduling plans](#scheduling-plans)
  - [Mappings](#mappings)
    - [Plans](#plans-1)
    - [Features](#features)

Tier supports [PriceOps Pricing Models](/TODO) using the `pricing.json` schema. Use
Pricing Models to define plan, feature, and pricing relationships for _pushing_
to a target billing engine.

## Introduction

Tier Pricing Models are expressed as an _infinite_ “human JSON” file referred to
as `pricing.json`, which can be viewed as the whole file, or slices of the file.
HuJSON is a superset of JSON that allows comments and trailing commas. The
_infinite_ nature of a `pricing.json` allows for working with the whole file, or a slice
of the file at a time. This makes `pricing.json`files easy to maintain while
staying read/writable by both humans and machines.

TODO: address how to deal with large pricing.json. For simple cases, you can
edit the policy file by hand in the admin panel. Organizations with more complex
needs can use the API to automatically update rules from software.

## Pricing syntax

The default pricing model is an empty one without any plans, features, or
prices. If `tier pull` is run on a fresh Stripe account, this is what the output
will be:

```json
{{ model `{
  "plans": {},
}` -}}
```

### Plans

define a proper plan name

- provide the prefix `plan:`
- provide a name after the prefix that contains only `[\w\d_]+`,
- provide a version after the name that starts with `@` followed by alphanumeric charters only.

Each plan name must start with the prefix `plan:`, 

Plans describe and define a collection of one or more entitlements
called "features".  The minimal plan contains _only_ a uniquely identifiable
key. A minimal plan looks like:

```json
{{ model `{
  "plans": {
    "plan:empty@0": {},
  },
}` -}}
```

The minimal doesn't allow the user to do anything and is referred to as the
_cancel plan_, because it denys access to all features.

## Pricing samples  

  * wws: develop simple plans
  * wws: create tailored plans, entitlements, and limits for large customer
  * wws: develop complex, incentive-based plans 
  * wws: rapidly tweak and test new pricing plans with limited sets of customers
  * wws: introduce new pricing models without inadvertently disrupting existing customers billing expectations. (stripe prevents this already, but we should still talk about it)

## Scheduling plans






























<!-- 




# Pricing Models (pricing.json)



# Introduction



- push a "hello, world!"
  - 


# Safety

- grandfather customers
- ship silently
- build and ship custom plans

# Quick Reference

working with one, many, or all plans
- push [... planID]
- pull [... planID]










































s: 
c: 
q: how do I use Tier to "terraform" Stripe?
a: these ways
  - setup tier with stripe
    - install tier
    - setup stripe
    - connect with test key
  - test with example pricing model
    - paste this example pricing model
    - `tier push`
      - ensure all succeeded (it looks like this pic:)
      - ensure product looks right in Stripe
  - Learn the pricing json structure and syntax


q: how do I build a pricing model

actions:

- grandfather customers
- ship new plans safely 
- build custom plan for a customer
- ship a hello world
- install tier
- push plan
- push multiple plans
- push duplicate plan
- pull all plans
- pull selected plans
- verify product in stripe
- verify price in stripe
- pj.mode -> price.tiers_mode
- pj.interval -> price.billing_interval
- pj.aggregate -> price.aggregate
- pj.aggregate.sum
- pj.aggregate.max
- pj.aggregate.min
- pj.aggregate.sum
- pj.aggregate.last
- pj.aggregate.last_seen
- tier push (duplicate error)
- tier connect
- stripe live keys
- stripe test keys
- stripe perms (lock down for prod)
- tier explain

s: we know we have a pricing plan and feature management issue
c: pricing is chaos at my company
q: how do I keep my pricing plans and features in order?
a: use pricing json

q: how do I start using pricing json?
a: these steps:
  - install `tier`
  - connect using `tier connect`
  - write this pricing json to a file: `plans: { ... some simple single feature plan ... }`
  - `tier push`
    - check it has succeeded
      - success lines look like this: 
      - failures look like this:
    - open links
      - observe product data
      - observe price data

s: we don't have pricing
c: we want to start charging
q: what should I do?
a: create a pricing json

s: we already have pricing (in Stripe)
c: we don't want to manually write a pricing json
q: how do we import into pricing json (from Stripe)?
a: coming soon

s: we already have pricing (in ad-hoc home-grown thing)
c: we don't want to manually write a pricing json
q: how do we import?
a: write custom migration script :/

# Pricing Models

Pricing models are expressed as a single "human JSON" file. HuJSON is a superset
of JSON that allows comments and trailing commas.

The pricing model file contains the following sections related to pricing and packaging:

* `plans`, the pricing plans themselves.
* `plans.[PlanID].features`, the features included in a plan, along with the pricing for the feature relative to the
plan it is in.

An example pricing model file looks like:

```json
{{ model `{
        "plans": {
                "plan:free@0": {
                        "title": "Todo (Free)",
                        "features": {
                                "feature:todo:lists": {
                                        "tiers": [{"upto": 5}],
                                },
                        },
                },

                "plan:pro@0": {
                        "title": "Todo (Pro)",
                        "features": {
                                "feature:support:email": {
                                "base": 9900, // $99.00
                                },
                                "feature:todo:lists": {
                                        "tiers": [{"upto": 100}],
                                },
                        },
                }
        }
}`
}}
```

## Features

Every feature in your application is identified with a string starting with
`feature:`. The rest of the string can contain any arbitrary identifier that you
use internally to reference a feature. You can think of this as a feature flag
for restricting usage to paid features and tracking how much of something a
customer has consumed.

The feature name should identify what thing the user is trying to do or consume,
which you might bill for (or at least track). For example:

* `feature:send-message`
* `feature:files-stored`
* `feature:bandwidth-gb`
* `feature:generate-pdf`
* `feature:read-only-seat`
* `feature:requests`

An example of a feature looks like:

```json
{{ hujson `{
        // Customers subscribed to a plan containing this
        // feature can have up to 10 todo lists in the
        // their account at any time.
        "feature:todo:lists": {
                "aggregate": "last",
                "tiers": [{"upto": 10, "base": 900}],
        },
}` }}
```

### Tiers

Each feature has a set of 0 or more "tiers", which define the prices and limits
for use of that feature. Each tier has the following fields:

* `upto`, The upper limit of the current tier. If not specified, then the tier is unbounded.
* `price`, The price per unit of consumption within this tier.
* `base`, The price that a user is charged immediately when reaching this tier of usage. Defaults to 0.

The simplest "free, unlimited" tier is `{}`. Since it doesn't specify anything,
it uses the defaults: up to Infinity, $0.00 per unit, $0.00 base price.

For example, if we want to say that streaming a song on our platform costs $1
each for the first 100, and then $0.50 thereafter, we could do:

```json
{{ model `
{
  "plans": {
    "plan:streamer@123": {
      "features": {
        "feature:song-stream": {
          "tiers": [
            // $1 for the first 100
            { "upto": 100, "price": 100 },
            // $0.50 thereafter
            { "price": 50 }
          ]
        }
      }
    }
  }
}
` }}
```

A feature with no tiers is billed based "licensing" vs. usage.

```json
{{ model `
{
  "plans": {
    "plan:streamer@123": {
      "features": {
        "feature:song-stream": {
          "tiers": [
                { "price": 100, "upto": 100 },
                { "price": 50 },
          ],
        },
        // Explicitly disabled. Any usage will be treated as overage.
        "features:song-download": {
          "tiers": []
        }
      }
    }
  }
}
` }}
```

### Interval

| Value                | Meaning                        |
| -------------------- | ------------------------------ |
| `@daily`             | The feature is bills each day  |
| `@monthly` (default) | The feature bills each month   |
| `@yearly`            | The feature bills each year    |
| `@quarterly`         | The feature bills each quarter |

### Currency

The `currency` field specifies the currency the features prices are in. The
default currency is `usd`.

### Base

The `base` field is a positive integer that specifies the base price for
features without tiers.  It is an error to specify both the `base` field and a
non-empty tiers

### Aggregate

The `aggregate` field specifies the function used to determine the usage
multiplier for calculating the total price at billing time.

TODO: more words

| Value            | Meaning                                                                                                                              |
| ---------------- | ------------------------------------------------------------------------------------------------------------------------------------ |
| `sum`  (default) | Multiply total usage by the sum of all values reported during the period                                                             |
| `max`            | Multiply total usage by the largest value reported during the period                                                                 |
| `min`            | Multiply total usage by the smallest value reported during the period                                                                |
| `recent`         | Multiply total usage by the last value seen during the current period. If no usage was reported during the period, then `0` is used. |
| `perpetual`      | Multiply total usage by the last value seen, across all periods.                                                                     |

#### Mode

The `mode` field specifies how to determine a bill using `tiers`.

| Value                 | Meaning                                           |
| --------------------- | ------------------------------------------------- |
| `graduated` (default) | Per unit pricing changes in _ranges_ over usage   |
| `volume`              | Per unit pricing changes for all units over usage |

For a more detailed explanation of graduated vs volume-based pricing, head to
[Graduated vs. Volume Based Pricing](/TODO)















* safely push plans
* reduce toil in stripe (give zero-clicks)
  * wws: 
* think in features
  * wws: 
* iterate on pricing
* commit plans to code
  * wws: ???
* custom tailor plans for individual customers
* define per seat/unit licensing
* define consumption-based pricing (volume/graduated)

-->


<!--
	hasTiers := len(v.Tiers) > 0

	if hasTiers {
		if v.Base > 0 {
			return nil, fmt.Errorf("a non zero base is not supported for tiered pricing")
		}

		slices.SortFunc(v.Tiers, func(a, b schema.Tier) bool {
			return a.Upto < b.Upto
		})

		pp.UnitAmount = nil
		pp.BillingScheme = ptr("tiered")
		pp.Recurring.UsageType = ptr("metered")
		pp.Recurring.AggregateUsage = ptr(string(v.Aggregate))
		pp.TiersMode = ptr(string(v.Mode))
	}

	for _, t := range v.Tiers {
		pt := &stripe.PriceTierParams{
			UnitAmount: ptr(t.Price),
			FlatAmount: ptr(t.Base),
		}

		switch t.Upto {
		case 0:
			return nil, fmt.Errorf("invalid tier %v; zero upto reserved for future use", t)
		case schema.Inf:
			pt.UpToInf = ptr(true)
		default:
			pt.UpTo = ptr(t.Upto)
		}

		pp.Tiers = append(pp.Tiers, pt)
	} -->

<!-- 
	pp := &stripe.PriceParams{
		Params: stripe.Params{
			Context: ctx,
			Metadata: map[string]string{
				"tier.plan":    planID,
				"tier.feature": v.ID,
			},
		},

		Product:   ptr(MakeID(planID)),
		Currency:  ptr(v.Currency),
		LookupKey: ptr(MakeID(planID, v.ID)),
		Nickname:  ptr(v.ID),

		BillingScheme: ptr("per_unit"),
		UnitAmount:    ptr(v.Base),
		Recurring: &stripe.PriceRecurringParams{
			Interval:  ptr(string(interval)),
			UsageType: ptr("licensed"),
		},
	}

-->

---

## Mappings

### Plans

| Tier Plan                         | Stripe Product Field            | Transform (if any) |
| --------------------------------- | ------------------------------- | ------------------ |
| `plans[PlanID]`                   | `product.id`                    | makeID(PlanID)     |
| `plans[PlanID]`                   | `product.metadata["tier.plan"]` |                    |
| `plans[PlanID].title` OR `PlanID` | `product.title`                 |                    |
| `"service"`                       | `product.type`                  |                    |

### Features

For each feature in a plan, an equivalent price is created in Stripe, each with its own price ID.

| Tier Feature | Stipe Price Field                                  | Transform                              |
| ------------ | -------------------------------------------------- | -------------------------------------- |
| -            | `makeID(join(PlanID, "__", FeatureID))`            |                                        |
| `currency`   |                                                    |                                        |
| `title`      | `plans[PlanID].features[PlanID].title` OR `PlanID` |                                        |
| `price.type` | `"service"`                                        | Currently, only services are supported |


<--

* open pricing.json
* add plan:free@0 (without title)
* add feature:base (without title)
* add 



* you have created a new pricing model, pushed it to Stripe, verified it mapped correctly, added it to a customers schedule, and verified it worked


* push your completed pricing model

-->