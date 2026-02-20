# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

azure2aws is a Go CLI tool that authenticates to AWS using Azure AD SAML via browser automation (chromedp). It launches a Chromium-based browser (Chrome, Edge, or Chromium), lets the user sign in to Azure AD manually, passively captures the SAML response from the network traffic, then uses AWS STS AssumeRoleWithSAML to obtain temporary credentials written to `~/.aws/credentials`.

## Build Commands

```bash
make build      # Compile binary with version from version.txt
make sign       # Build + codesign (macOS)
make install    # Sign + copy to /usr/local/bin/
make clean      # Remove compiled binary
```

Version bumping:
```bash
make bump-patch  # 1.0.x
make bump-minor  # 1.x.0
make bump-major  # x.0.0
```

There are no tests, linting, or CI/CD configured.

## Architecture

The entire application lives in a single file: **main.go** (~610 lines). The Go module is named `acn2aws`.

### Core Flow (main function)

1. Parse CLI args (URL and optional profile name), or load last-used URL from `~/.acn2aws/config`
2. Detect display environment (Linux: check DISPLAY/WAYLAND_DISPLAY)
3. Detect browser path via `GetBrowserPath()` (Chrome > Edge > Chromium priority)
4. Launch browser via `runBrowserSession()` using chromedp — manages process lifecycle automatically
5. Enable passive network monitoring via `chromedp.ListenTarget` — captures POST requests to `signin.aws.amazon.com`
6. Navigate browser to the Azure sign-in URL; user authenticates manually
7. Wait for SAML response on channel (5-minute timeout), or detect browser close
8. Base64-decode and XML-parse the SAML assertion to extract AWS role ARNs
9. Call AWS STS AssumeRoleWithSAML to get temporary credentials
10. Write credentials to `~/.aws/credentials` under the chosen profile

### Key Components

- **`runBrowserSession()`** — Core browser automation function using chromedp. Creates an `ExecAllocator` with the detected browser, sets up passive network listeners via `chromedp.ListenTarget`, navigates to the Azure URL, and waits for SAML capture on a channel.
- **`GetBrowserPath()`** — Detects installed Chromium-based browsers with platform-specific paths. Priority: Chrome > Edge > Chromium. Returns `(string, error)` with actionable error listing searched paths.
- **`extractSAMLFromPostData()`** — Standalone function that parses URL-encoded POST data to extract the SAMLResponse field.
- **SAML parsing** — XML structs (SAMLResponse, Attribute, AttributeValue) decode the SAML assertion and extract role/provider ARN pairs.
- **AWS credential management** — Uses aws-sdk-go-v2 STS client (hardcoded to us-east-1) and writes INI-format credentials via gopkg.in/ini.v1.
- **Network monitoring** — `chromedp.ListenTarget` callback type-switches on `*network.EventRequestWillBeSent`, matching POST to `signin.aws.amazon.com`. Falls back to `network.GetRequestPostData` in a goroutine for large payloads.

### Concurrency

- Main goroutine handles CLI interaction and orchestration
- chromedp manages its own internal goroutines for CDP communication
- Goroutines spawned in `ListenTarget` callback for `network.GetRequestPostData` fallback
- SAML result communicated via buffered channel (`samlCh`)

### Platform-Specific Code

Browser path detection (`GetBrowserPath`) branches on `runtime.GOOS` for macOS, Linux, and Windows. Linux display detection checks `DISPLAY` and `WAYLAND_DISPLAY` environment variables.

### Hardcoded Configuration

- SAML wait timeout: 5 minutes
- AWS STS region: us-east-1
- Config file: `~/.acn2aws/config`

## Active Technologies
- Go 1.24 + chromedp, cdproto/network (transitive), aws-sdk-go-v2, gopkg.in/ini.v1
- `~/.aws/credentials` (INI), `~/.acn2aws/config` (plain text)

## Recent Changes
- 001-cross-browser-automation: Replaced manual CDP/WebSocket code with chromedp library. Added Edge browser detection. Improved error messages. Removed gorilla/websocket dependency.
