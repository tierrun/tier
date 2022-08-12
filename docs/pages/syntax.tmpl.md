# Pricing Model Definitions

Tier helps manage pricing plans, features, their prices, and how price may
evolve as your customers consume your products.

The plans, features, and prices are expressed in a single "human JSON" definition file.


## Introduction

Tier pricing model definitions are expressed as a single “[human
JSON](https://github.com/tailscale/hujson)” Pricing JSON file. HuJSON is a
superset of [JSON](https://www.json.org/json-en.html) that allows comments and
trailing commas. This makes Pricing JSON files easy to maintain while
staying read/writable by both humans and machines.

A pricing model has a single section `plans` that contains a list of plans you want to sell, have sold, or are currently selling to customers.

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