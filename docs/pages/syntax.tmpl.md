# Pricing Model Definitions

Tier helps manage pricing models, prices, features, and more with the Pricing
Model Syntax.

## Introduction

Tier pricing model definitions are expressed as a single “[human
JSON](https://github.com/tailscale/hujson)” definition file. HuJSON is a
superset of [JSON](https://www.json.org/json-en.html) that allows comments and
trailing commas. This makes ACL files easy to maintain while staying
read/writable by both humans and machines.

## Plans

The plans section lets you define plans for later adding to customer schedules
using your billing backend.

### Plan Identifiers

Plan identifiers are unique ids that specify a plan at a specific version.

A plan identifier looks like `plan:free@1`.

A plan definition looks like:

```json
{{ model `{
        "plans": {
                "plan:free@0": {
                        "title": "Todo (Free)", // human-friendly title

                        // Customers subscribed to this plan have access to 5
                        // lists. Each of the 5 lists are free.
                        "features:todo:lists": {
                                "tiers": [{"upto": 5}],
                        },
                },

                // later it is decided the free plan should allow customers to
                // invite one friend.
                "plan:free@1": {
                        // Customers subscribed to this plan have access to 5
                        // lists. Each of the 5 lists are free.
                        "features:todo:lists": {
                                "tiers": [{"upto": 5}],
                        },
                        "features:invites": {
                                "tiers": [{"upto": 1}],
                        },
                },

                "plan:pro@0": {
                        // customers
                        // ...
                }
        }
}` }}
```

