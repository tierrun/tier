<p align="center">
  <img src="https://uploads-ssl.webflow.com/61e0906dfb20ab2b1c79f6af/638e175ae356c54fe57a7579_IMG_8588.png" />
 </p>


# Pricing as Code

`tier` is a tool that lets you define and manage your SaaS application's pricing model in one place (pricing.json). 

Tier will handle setting up and managing Stripe in a way that is much more friendly for SaaS and consumption based billing models. Tier's SDK can then be implemented for access checking, metering/reporting, and more.

  [![GPLv3 License](https://img.shields.io/github/license/tierrun/tier?style=for-the-badge)](https://opensource.org/licenses/)



## Docs and Community
- [Documentation is available here](https://tier.run/docs)
- Join our Slack here: [<img src="https://img.shields.io/badge/Slack-4A154B?style=for-the-badge&logo=slack&logoColor=white" />](https://join.slack.com/t/tier-community/shared_invite/zt-1blotqjb9-wvkYMo8QkhaEWziprdjnIA)

# Key Features and Capabilities
- Manage your features, plans and their pricing in one place
- On demand test environments and preview deployments allow you to work with confidence
- Create custom plans and variants as needed for specific customers or tests
- Stripe is kept in sync and fully managed by Tier
- Access Checking and Entitlements are handled by the Tier SDKs 

## How to use Tier

1. [Install Tier CLI](#install)
2. [Create your first pricing.json](https://model.tier.run) and `tier push` to your dev or live environment
3. [Get a Tier SDK and add it](https://www.tier.run/docs/sdk/) to enable Access Checks and Metering

You can see and example here: [Tier Hello World!](https://blog.tier.run/tier-hello-world-demo)

<p align="center">
  <img src="https://uploads-ssl.webflow.com/61e0906dfb20ab2b1c79f6af/637c39698d3ba183d982e32a_Screenshot%202022-11-21%20at%2010.43.54%20PM.png">
</p>

## Install

### Homebrew (macOS)

```
brew install tierrun/tap/tier
```
### Binary (macOS, linux, Windows)

Binaries for major architectures can be found at [here](https://tier.run/releases).

### Go (most operating systems and architectures)

If go1.19 or later is installed, running or installing tier like:

```
go run tier.run/cmd/tier@latest
```

or

```
go install tier.run/cmd/tier@latest
```


## Authors

- [@bmizerany](https://www.github.com/bmizerany)
- [@isaacs](https://www.github.com/isaacs)
- [@jevon](https://www.github.com/jevon)

