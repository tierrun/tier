# Tier <!-- omit in toc -->

- [About](#about)
- [Install](#install)
  - [Homebrew (macOS)](#homebrew-macos)
  - [Binary (macOS, linux, Windows)](#binary-macos-linux-windows)
  - [Go (most operating systems and architectures)](#go-most-operating-systems-and-architectures)
- [Further Reading](#further-reading)

## About
tier is a tool that lets you define and manage your SaaS application's pricing model in one place (pricing.json). Tier will handle setting up and managing Stripe in a way that is much more friendly for SaaS and consumption based billing models. Tier's SDK can then be implemented for access checking, metering/reporting, and more.

[More detail and documentation is available on the Tier website.](https://tier.run/docs)

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

## Further Reading

https://tier.run/docs
