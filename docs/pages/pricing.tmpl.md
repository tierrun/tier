<!-- 
1. install tier
1. signup for stripe
1. connect tier to stripe
2. follow the link to test
3. see `tier pull` output (empty pricing.json)
4. make simple plan (licensed with no base)
-->

# Using Tier <!-- omit in toc -->

- [Getting Started](#getting-started)
  - [Install](#install)
  - [Connect](#connect)
  - [Hello Pricing](#hello-pricing)
- [Pricing Models](#pricing-models)
  - [Introduction](#introduction)
  - [Pricing model syntax](#pricing-model-syntax)
  - [Plans](#plans)
  - [Features](#features)
    - [Licensed](#licensed)
    - [Metered](#metered)
    - [Limits](#limits)

# Getting Started


> TODO: This section will guide you through pushing your first minimal pricing model to Stripe.
## Install

> TODO
## Connect

- create stripe account (if not already there)
- install tier
- connect: `tier connect`
- verify: `tier pull`

> TODO
## Hello Pricing

> TODO
# Pricing Models


## Introduction

TODO

## Pricing model syntax

Pricing Model Definitions are expressed in a single HuJSON file. HuJSON is a
superset of JSON that allows for comments and trailing commas. This allows
humans to maintain the file, while also being easy for machines to read.

> The term `pricing.json` is short for Pricing Model Definition File, and is
used frequently below.

An example `pricing.json`:

```json
{{ model `{
  "plans": {
    "plan:free@0": {
      "features": {
        // free plan users get 1 free Todo list
        "feature:todo:lists": {
          "tiers": [{"upto": 1}],
        },

        // free plan users work alone
        "feature:todo:seats": {}
      },

      "plan:pro@0": {
        "features": {
          // pro plan users get 10 Todo list for $9/mo
          "feature:todo:lists": {
            "tiers": [{"upto": 10, "base": 900}],
          },

          // pro plan users may have up to 2 more seats
          // at no additional charge
          "feature:todo:seats": {
              "tiers": [{"upto": 2}]
          },
        },
      },

      // A special enterprise plan for the ACME company
      "plan:acme@0": {
        "features": {
          // Acme doesn't worry about spend on todo lists, but instead wants to
          // buy based on seats at a known cost they can measure: seats.

          // unlimited free lists
          "feature:todo:lists": {
            "tiers": [{}],
          },

          "feature:todo:seats": {
            "mode": "volume",
            "aggregate": "last",
            "tiers": [
              {"upto": 100, "price": 900}, // $9/mo until more than 100 users
              {"upto": 500, "price": 700},
              {"price": 400},
            ],
          },
        },
      },
    }
  }
}` -}}
```

## Plans

The `plans` section includes zero or more plans, keyed on their plan ID.

An example plan looks like:


```json
{{ model `{
  "plans": {"plan:free@0": {
    "features": { "feature:all": {} },
  }},
}` -}}
```


Plan IDs follow the format: `plan:<name>@<version>`. Both the name and version
must contain at least one valid character. Valid name characters are in the
alphanumeric range, the `_`, and the `:`.  Valid version characters are in the
alphanumeric range, or the `_`.

An example plan looks like:

The `title` field specifies an optional human-friendly name for use in invoicing, pricing pages, etc. If no title is specified, the default title is the plan ID.

The `features` section specifies any features subscribers
of the plan are entitled to, and the pricing structure the subscriber will have
agreed to be selecting the plan.

## Features

The features section provides the means to:

- Specify the _features_ subscribers of the plan are entitled to.
- Specify the _pricing structure_ subscribers of the plan have agreed to per feature.

Each feature is keyed on a feature ID. Feature IDs follow the format:
`feature:<name>`. The name must contain at least one valid character. Valid name
characters are in the alphanumeric range, the `_`, and the `:`.

An example `feature` definition looks like:

```json
{{ hujson `{
  "feature:seats": {
    "interval": "@monthly",
    "aggregate": "perpetual",
    "currency": "eur",
    "mode": "volume",
    "tiers": [
      {"upto": 10, "price": 1000},
      {"upto": 50, "price": 700},
      {"price": 400},
    ]
  }
}` -}}
```

By default, a feature without any fields or tiers set, is a free feature with an
unlimited limit.

```json
{{ hujson `{
  "feature:seats": {},
}` -}}
```


* `currency`
* `base`
* `aggregate`
* `mode`
* `tiers`

### Licensed

### Metered

### Limits

The `features` is used to list each feature, and it's pricing for the plan it is
in.




