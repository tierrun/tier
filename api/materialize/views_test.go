package materialize

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/tailscale/hujson"
	"golang.org/x/exp/slices"
	"kr.dev/diff"
	"tier.run/client/tier"
	"tier.run/control"
	"tier.run/refs"
)

func TestPricingHuJSON(t *testing.T) {
	data := []byte(`{
		"plans": {
			"plan:example@1": {
				"title": "Just an example plan to show off features",
				"features": {
					"feature:graduated": {
						"mode": "graduated",
						"tiers": [
							{ "upto": 10 },
							{ "upto": 20, "price": 100 },
							{ "price": 50 }
						],
					}
				}
			},
			"plan:example@2": {
				"title": "Just an example plan to show off features part duex",
				"features": {
					"feature:base": {
						"base": 100,
					},
				},
			},
		}
	}`)

	got, err := FromPricingHuJSON(data)
	if err != nil {
		t.Fatal(err)
	}
	slices.SortFunc(got, func(a, b control.Feature) bool {
		return a.Less(b.FeaturePlan)
	})

	want := []control.Feature{
		{
			PlanTitle:   "Just an example plan to show off features part duex",
			Title:       "feature:base@plan:example@2",
			FeaturePlan: refs.MustParseFeaturePlan("feature:base@plan:example@2"),
			Currency:    "usd",
			Interval:    "@monthly",
			Mode:        "graduated", // defaults
			Aggregate:   "sum",       // defaults
			Base:        100,
		},
		{
			PlanTitle:   "Just an example plan to show off features",
			Title:       "feature:graduated@plan:example@1",
			FeaturePlan: refs.MustParseFeaturePlan("feature:graduated@plan:example@1"),
			Currency:    "usd",
			Interval:    "@monthly",
			Mode:        "graduated",
			Aggregate:   "sum",
			Tiers: []control.Tier{
				{Upto: 10, Price: 0, Base: 0},
				{Upto: 20, Price: 100, Base: 0},
				{Upto: tier.Inf, Price: 50, Base: 0},
			},
		},
	}

	diff.Test(t, t.Errorf, got, want)

	gotJSON, err := ToPricingJSON(got)
	if err != nil {
		t.Fatal(err)
	}

	wantJSON := []byte(`{
		"plans": {
			"plan:example@1": {
				"title": "Just an example plan to show off features",
				"features": {
					"feature:graduated": {
						"tiers": [
							{ "upto": 10 },
							{ "upto": 20, "price": 100 },
							{ "price": 50 }
						],
					}
				}
			},
			"plan:example@2": {
				"title": "Just an example plan to show off features part duex",
				"features": {
					"feature:base": {
						"base": 100,
					}
				}
			}
		}
	}`)

	diffJSON(t, gotJSON, wantJSON)
}

func diffJSON(t *testing.T, got, want []byte) {
	t.Helper()

	format := func(b []byte) string {
		b, err := hujson.Standardize(b)
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if err := json.Indent(&buf, b, "", "    "); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}

	diff.Test(t, t.Errorf, format(got), format(want))
}
