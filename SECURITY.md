# Security

## Reporting a vulnerability

Please report security issues **privately** instead of opening a public GitHub issue.

- Use **[GitHub Security Advisories](https://github.com/alexeyleshchenko/ai-gateway/security/advisories)** for this repository: *Report a vulnerability* if the feature is enabled.
- If advisories are unavailable, contact the repository maintainers through a private channel.

Include enough detail to reproduce or assess the issue (affected component, configuration, and impact). We will treat reports confidentially and coordinate a fix and disclosure timeline with you.

## Scope

We treat the following as **in scope** for coordinated disclosure when they affect a supported deployment of this gateway:

- Authentication or authorization bypass affecting `/v1/*` routes
- Exposure or theft of API keys, tokens, or secrets via the gateway process
- Remote code execution or unsafe deserialization via request handling
- Denial of service with a realistic high-impact scenario (not generic load testing)

**Out of scope** for private advisories (use regular [issues](https://github.com/alexeyleshchenko/ai-gateway/issues) instead): deployment mistakes (misconfigured Traefik, leaked `.env` on disk), provider outages, or issues in dependencies unless they require a gateway-side change.

## Supported versions

Security fixes are applied to the default branch (`main`) and released via the usual CI/CD and container tags (for example `main` on GHCR). Use the latest image or binary from `main` for deployments that need current fixes.

## General contributions

For non-security bug reports and feature work, see [CONTRIBUTING.md](CONTRIBUTING.md).
