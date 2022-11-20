name: "test"

on: pull_request: branches: ["*"]

jobs: {
	test: {
		"runs-on": "ubuntu-latest"
		steps: [{
			name: "Checkout"
			uses: "actions/checkout@v3"
		}, {
			name: "Setup Go"
			uses: "actions/setup-go@v3"
			with: {
				"go-version": "1.19"
			}
		}, {
			name: "Stroke Cache"
			id:   "stroke-cache"
			uses: "actions/cache@v3"
			with: {
				path: """
					~/.cache/stroke
					"""
				key: "stroke-cache"
			}
		}, {
			name: "Go Test"
			env: STRIPE_API_KEY: "${{ secrets.STRIPE_API_KEY }}"
			run: """
				go test -count=1 -v ./...
				"""
		}]
	}
}
