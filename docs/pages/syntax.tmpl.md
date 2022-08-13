# Pricing models

Tier helps manage pricing plans, features, their prices, and how price may
evolve as your customers consume your products.

## Pricing model syntax

Pricing models plans, features, and prices are expressed in a single "human JSON" definition file.

<span class="text-xs">_TODO: touch on HuJSON_ (nod to tailscale)</span>


The pricing model file has a single top-level section: `plans`.

Each plan is identified by a unique field in the `plans` section. This
identifier starts with `plan:` and contains a `name@version`. The name does not
need to be unique, but the version must be unique per name. The name must
contain only alphanumeric characters or the underscore (`_`). The version must
be only alphanumeric characters.

Example plan identifiers:

```
plan:free@0
plan:pro@99
plan:pro:acme@11
plan:welcome@9kk3j393DAF236
```




 contains the optional field `title`, and the section `features`.


## Plans

A billing plan re

### Plan Identifiers

Every billing plan that a user might be signed up for is identified with a string starting with 'plan:', and containing a @. The part before the @ is the "name". The part after the @ is the "version".

```
plan:free@2
plan:pro@13
plan:enterprise@custom-for-acme-signed-2022-05-13
plan:pro@trial-23
plan:basic@beta-user-discount-25
```

You can define as many versions of as many plans as you want in your pricing model. However, you may not change a given version of a plan, so to make changes you will create a new version of it. This makes it possible to experiment safely with pricing changes, and only update existing customers' plans when it makes sense for your application.

Think of your set of plans as an append-only set of the various ways you package your application and bill your customers.



Each plan contains these sections:

- `title`: A human-friendly title for the plan.
- `base`: A positive _starting_ price (in cents) to be billed per interval
- `interval`: The interval to bill this plan at.
- `features`: The list of features (entitlements) this plan includes, and any specific pricing and billing rules for them.
- `


A plan definition looks like:

```json
{{ model `{
        "plans": {
                "plan:free@0": {
                        "title": "Todo (Free)", // human-friendly title

                        "features": {
                                // Customers subscribed to this plan have access to 5
                                // lists. Each of the 5 lists are free.
                                "feature:todo:lists": {
                                        "tiers": [{"upto": 5}],
                                },
                        },

                },

                // later it is decided the free plan should allow customers to
                // invite one friend.
                "plan:free@1": {
                        "title": "Todo (Free)",
                        "features": {
                                "feature:todo:lists": {
                                        "tiers": [{"upto": 5}],
                                },
                                "feature:invites": {
                                        "tiers": [{"upto": 1}],
                                },
                        },
                },

                "plan:pro@0": {
                        "features": {
                                "feature:support:email": {},
                                "feature:todo:lists": {
                                        "tiers": [{"upto": 5}],
                                },
                                "feature:invites": {
                                        "tiers": [{"upto": 1}],
                                },
                        },
                }
        }
}` }}
```