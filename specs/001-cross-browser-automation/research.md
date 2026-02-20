# Research: Replace Manual CDP with Standard Browser Automation Library

**Date**: 2026-02-19
**Feature**: 001-cross-browser-automation

## Decision 1: Browser Automation Library

**Decision**: Use `chromedp` (`github.com/chromedp/chromedp`)

**Rationale**:
- Pure Go, zero external runtime dependencies — preserves single-binary deployment
- Most mature Go CDP library (~12,700 GitHub stars, actively maintained, latest release v0.13.2 March 2025)
- Provides typed Go structs for all CDP events via `cdproto` sub-packages
- `ListenTarget` API supports truly passive network event monitoring (no interception)
- `ExecPath` option allows pointing at any Chromium-family binary (Chrome, Edge, Chromium)
- Automatic temp user-data-dir creation and cleanup — replaces our manual `os.MkdirTemp`
- `network.Enable().WithMaxPostDataSize()` ensures POST body available in events

**Alternatives considered**:

| Library | Verdict | Why rejected |
|---------|---------|--------------|
| rod (go-rod/rod) | Viable but inferior | Uses `HijackRequests` (active interception via Fetch domain) instead of passive monitoring. Slower maintenance cadence (last release July 2024). |
| playwright-go | Too heavy | Requires ~50MB Node.js runtime + 150-250MB browser downloads. Not pure Go. |
| tebeka/selenium | Not viable | WebDriver protocol has no network monitoring. Cannot capture POST data without a proxy. |
| Raw CDP (current) | Being replaced | ~400 lines of manual WebSocket/JSON handling. Works but fragile and duplicates what chromedp provides. |

## Decision 2: SAML Capture Mechanism

**Decision**: Use `chromedp.ListenTarget` + `network.EventRequestWillBeSent` + `network.GetRequestPostData`

**Rationale**:
- Direct replacement for current `handleMessage` + `readMessages` pattern
- `ListenTarget` callback receives typed `*network.EventRequestWillBeSent` events (no manual JSON parsing)
- `Request.HasPostData` boolean indicates POST body availability
- `Request.PostDataEntries` may contain inline data for small payloads
- `network.GetRequestPostData(requestID)` fetches full body for large payloads (fallback)
- Setting `WithMaxPostDataSize(1 << 20)` on `network.Enable()` increases inline threshold to 1MB
- Listener callback is synchronous — `GetRequestPostData` must be dispatched to a goroutine using `cdp.WithExecutor(ctx, target)`

**Current code mapping**:

| Current (raw CDP) | New (chromedp) |
|---|---|
| `ChromeDebugger.readMessages()` goroutine | `chromedp.ListenTarget(ctx, callback)` |
| `handleMessage()` JSON switch on method | Type switch on `ev interface{}` in callback |
| `Network.requestWillBeSent` string match | `*network.EventRequestWillBeSent` type assertion |
| `SendCommand("Network.enable", nil)` | `network.Enable().WithMaxPostDataSize(1<<20)` |
| `SendCommand("Page.navigate", ...)` | `chromedp.Navigate(url)` |
| `fetchPostData()` with manual JSON | `network.GetRequestPostData(id).Do(cdp.WithExecutor(ctx, target))` |
| `gorilla/websocket.Dial` + retry loop | `chromedp.NewExecAllocator` + `chromedp.NewContext` |
| Manual temp dir + `exec.Command` launch | Automatic via `ExecAllocator` (auto temp dir + cleanup) |
| `ChromeDebugger.Close()` kill process | `cancel()` on allocator context |

## Decision 3: Browser Detection and Priority

**Decision**: Reuse `GetChromePath()` pattern with Edge paths added, pass result to `chromedp.ExecPath()`

**Rationale**:
- chromedp's default behavior tries to find Chrome automatically, but doesn't search for Edge
- Our existing path-scanning logic is simple and works — just needs Edge paths added
- Pass the found path via `chromedp.ExecPath(path)` allocator option
- Priority order: Chrome > Edge > Chromium (per spec FR-002)

**Edge paths to add**:

| Platform | Path |
|----------|------|
| macOS | `/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge` |
| Linux | `/usr/bin/microsoft-edge`, `/usr/bin/microsoft-edge-stable` |
| Windows | `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`, `C:\Program Files\Microsoft\Edge\Application\msedge.exe` |

## Decision 4: Browser Close Detection

**Decision**: Use `chromedp.ListenBrowser` + `target.EventTargetDestroyed` combined with context cancellation

**Rationale**:
- `ListenBrowser` receives browser-level events including tab/target destruction
- Match `EventTargetDestroyed.TargetID` against our tab's target ID
- Context's `Done()` channel catches browser process exit (crash, kill, normal close)
- Replaces current behavior where `readMessages` goroutine exits on WebSocket error

## Decision 5: User Data Directory

**Decision**: Use chromedp's automatic temp directory (default behavior)

**Rationale**:
- chromedp creates `os.MkdirTemp("", "chromedp-runner")` automatically when no `UserDataDir` is specified
- Automatically cleaned up when allocator context is cancelled
- Replaces our manual `os.MkdirTemp("", "chrome-debug-*")` + no-cleanup (current code leaks temp dirs)
- Bonus: fixes the temp dir leak in the current implementation
