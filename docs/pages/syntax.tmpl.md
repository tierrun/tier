# Pricing JSON

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

## Plans

> TODO

## Features

The features section under a plan is a map of features keyed on a
[FeatureID](#featureid). Each feature if a HuJSON object grants access to a
feature.

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

<!-- 
			interval:  *"@monthly" | "@yearly" | "@weekly" | "@daily"
			currency:  *"usd" | string
			base:      *0 | (int & >=0)
			aggregate: *"sum" | "max" | "min" | "last"

			mode: *"graduated" | "volume"

			// The `tiers` section lets you define pricing that
			// varies based on consumption. 
			tiers: [...{
-->

### Interval

| Value                | Meaning                        |
| -------------------- | ------------------------------ |
| `@daily`             | The feature is bills each day  |
| `@monthly` (default) | The feature bills each month   |
| `@yearly`            | The feature bills each year    |
| `@quarterly`         | The feature bills each quarter |

### Currency

The `currency` field specifies the currency the features prices are in. The default currency is `usd`.

### Base

The `base` field is a positive integer that specifies the base price for
features without tiers.  It is an error to specify both the `base` field and a
non-empty tiers

### Aggregate

The `aggregate` field specifies the function used to determine the usage multiplier for calculating the total price at billing time.

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


### Tiers

TODO-