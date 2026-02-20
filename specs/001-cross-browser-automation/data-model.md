# Data Model: Replace Manual CDP with Standard Library

**Date**: 2026-02-19
**Feature**: 001-cross-browser-automation

## Entities

This feature modifies internal code structure only. No new user-facing data entities are introduced. The existing data entities (SAML Response, AWS Credentials, config file, credentials file) are unchanged.

### Modified Entity: Browser Session (internal)

**Before**: `ChromeDebugger` struct with manual state management

```
ChromeDebugger
├── cmd          *exec.Cmd         (Chrome process handle)
├── wsURL        string            (WebSocket URL)
├── conn         *websocket.Conn   (WebSocket connection)
├── debugPort    int               (hardcoded 9222)
├── samlResponse string            (captured SAML)
├── requestID    int               (CDP message counter)
├── mu           sync.Mutex        (protects shared state)
├── done         chan struct{}      (signal channel)
└── requestIDs   map[string]bool   (tracked request IDs)
```

**After**: chromedp-managed context + channel-based SAML capture

```
Browser Session (managed by chromedp)
├── allocCtx     context.Context       (ExecAllocator — browser process lifecycle)
├── ctx          context.Context       (chromedp context — tab/target lifecycle)
├── cancel       context.CancelFunc    (cleanup trigger)
└── samlCh       chan string            (SAML response delivery)

chromedp internally manages:
├── Browser process spawning and killing
├── WebSocket connection and CDP message routing
├── Temp user-data-dir creation and cleanup
├── Request ID generation
└── Event dispatching to ListenTarget/ListenBrowser callbacks
```

### Unchanged Entities

- **SAML Response**: Base64-encoded XML assertion. Captured from POST body to `signin.aws.amazon.com`. Parsed via `encoding/xml` into existing `SAMLResponse` / `Assertion` / `Attribute` structs. No changes.
- **AWS Credentials**: `AWSCredentials` struct (AccessKeyID, SecretAccessKey, SessionToken, Expiration). Written to `~/.aws/credentials` via `gopkg.in/ini.v1`. No changes.
- **Config**: `~/.acn2aws/config` with `last_url=<url>`. No changes.

## Dependency Changes

| Dependency | Action |
|---|---|
| `github.com/chromedp/chromedp` | **Add** — browser automation |
| `github.com/chromedp/cdproto` | **Add** (transitive) — CDP protocol types |
| `github.com/gorilla/websocket` | **Remove** — replaced by chromedp internals |
