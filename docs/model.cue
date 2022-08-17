#PlanName:    =~"^plan:[a-zA-Z0-9_:]+@[a-zA-Z0-9]+$"
#FeatureName: =~"^feature:[a-zA-Z0-9_:]+$"

// OPI is an optional positive integer which defaults to 0. Unlike a unit, OPI
// has a default, and cannot exceed 9223372036854775807.
#OPI: *0 | int64 & >=0

#Model: {
	// plans are the plans offered for your service. Each plan is
	// identified by a PlanName. A valid PlanName has the prefix ("plan:"), a
	// name, and a version prefixed with ("@").
	//
	// Examples of a valid PlanName are:
	//
	//  - plan:free@0
	//  - plan:pro@1a4cX
	//  - plan:custom@ccc
	plans: [#PlanName]: {
		// title is an optional human-friendly title for the plan. It
		// replaces the PlanName in the UI, invoices, etc.
		//
		// Unlike the plan id, it is not a requirement for the title
		// to be unique across plans.
		title: *"" | string

		// TODO: this should expand to a base feature
		base: #OPI

		// features defines the features that are available for
		// customers subscribed to this plan. Each feature defines their
		// own prices, limits, and billing intervals.
		//
		// Feature names must be prefixed with ("feature:") followed by
		// alphanumeric characters, or underscores, or colons.
		features: [#FeatureName]: {
			interval: *"@monthly" | "@yearly" | "@weekly" | "@daily"
			currency: *"usd" | string

			base:      #OPI
			aggregate: *"sum" | "max" | "min" | "recent" | "perpetual"

			mode: *"graduated" | "volume"

			// The `tiers` section lets you define pricing that
			// varies based on consumption. 
			tiers: [...{
				base:  #OPI
				price: #OPI
				upto:  #OPI
			}]
		}
	}
}

#Model
