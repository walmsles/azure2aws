# Quickstart: Replace Manual CDP with Standard Library

**Date**: 2026-02-19
**Feature**: 001-cross-browser-automation

## Prerequisites

- Go 1.23+
- A Chromium-family browser installed (Chrome, Edge, or Chromium)
- Graphical display environment

## Build and Run

```bash
make build
./azure2aws <azure-signin-url> [profile-name]
```

No change from current workflow. The library swap is internal.

## What Changed

The `ChromeDebugger` struct and ~400 lines of manual CDP/WebSocket code in `main.go` are replaced by `chromedp` library calls. The `gorilla/websocket` dependency is removed. Edge browser paths are added to `GetChromePath()`.

## Key Code Patterns

### Browser launch (before)
```go
cd.cmd = exec.Command(chromePath, args...)
cd.cmd.Start()
// manual HTTP GET to /json, parse targets, websocket.Dial(wsURL)
```

### Browser launch (after)
```go
opts := append(chromedp.DefaultExecAllocatorOptions[:],
    chromedp.ExecPath(browserPath),
    chromedp.Flag("headless", false),
)
allocCtx, cancel := chromedp.NewExecAllocator(ctx, opts...)
ctx, cancel := chromedp.NewContext(allocCtx)
```

### SAML capture (before)
```go
// Background goroutine reads raw WebSocket messages
// Manual JSON parsing, string matching on "Network.requestWillBeSent"
// Manual extraction of postData from request params
```

### SAML capture (after)
```go
chromedp.ListenTarget(ctx, func(ev interface{}) {
    if e, ok := ev.(*network.EventRequestWillBeSent); ok {
        // Typed struct, no JSON parsing needed
        if e.Request.Method == "POST" && strings.Contains(e.Request.URL, "signin.aws.amazon.com") {
            // Extract SAML from POST data
        }
    }
})
```

## Verification

```bash
# Build
make build

# Test with Chrome
./azure2aws https://myapps.microsoft.com/signin/AWS/xxxxx

# Test with Edge (if Chrome not available)
# Tool auto-detects Edge when Chrome/Chromium absent

# Verify credentials written
aws sts get-caller-identity --profile <profile>
```
