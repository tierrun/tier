#PlanID:    =~"^plan:[a-zA-Z0-9_:]+@[a-zA-Z0-9]+$"
#FeatureID: =~"^feature:[a-zA-Z0-9_:]+$"

#Model: {
	// plans are the plans offered for your service. Each plan is
	// identified by a PlanID. A valid PlanID has the prefix ("plan:"), a
	// name, and a version prefixed with ("@").
	//
	// Examples of valid PlanIDs are:
	//
	//  - plan:free@0
	//  - plan:pro@1a4cX
	//  - plan:custom@ccc
	plans: [#PlanID]: {
		// title is an optional human-readable title for the plan. In
		// providers such as Stripe, this will be used as the product
		// name displayed to the customer in their invoices, or on
		// pricing pages.
		title: *"" | string

		// 
		base: int & >=0

		// features defines the features that are available for
		// customers subscribed to this plan. Each feature defines their
		// own prices, limits, and billing intervals.
		//
		// Feature names must be prefixed with ("feature:") followed by
		// alphanumeric characters, or underscores, or colons.
		features: [#FeatureID]: {
			// interval
			interval:  *"@monthly" | "@yearly" | "@weekly" | "@daily"
			currency:  *"usd" | string
			base:      *0 | (int & >=0)
			aggregate: *"sum" | "max" | "min" | "last"

			mode: *"graduated" | "volume"

			// The `tiers` section lets you define pricing that
			// varies based on consumption. 
			tiers: [...{
				base:  *0 | (int & >=0)
				price: *0 | (int & >=0)
				upto:  *0 | (int & >=0)
			}]
		}
	}
}

#Model
