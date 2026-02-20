# Implementation Plan: Replace Manual CDP with Standard Browser Automation Library

**Branch**: `001-cross-browser-automation` | **Date**: 2026-02-19 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/001-cross-browser-automation/spec.md`

## Summary

Replace ~400 lines of hand-rolled CDP/WebSocket code (`ChromeDebugger` struct, `readMessages`, `handleMessage`, raw `gorilla/websocket` connection) with the `chromedp` Go library. Add Edge browser detection on all platforms. Improve error messages for common failure scenarios. All external behavior (CLI interface, SAML capture, credential output) remains identical.

## Technical Context

**Language/Version**: Go 1.23
**Primary Dependencies**: chromedp (new), cdproto/network (new, transitive), aws-sdk-go-v2 (existing), gopkg.in/ini.v1 (existing). gorilla/websocket (removed).
**Storage**: `~/.aws/credentials` (INI), `~/.acn2aws/config` (plain text) — both unchanged
**Testing**: Manual end-to-end (no test suite exists; none added in this feature)
**Target Platform**: macOS, Linux, Windows (all with graphical display)
**Project Type**: Single file Go CLI
**Performance Goals**: N/A — user manually signs in; tool waits passively
**Constraints**: Must remain a single Go binary with no external runtime dependencies
**Scale/Scope**: Single-user CLI tool, ~850 lines in main.go

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Constitution is unconfigured (all placeholders). No gates to check. Proceeding.

**Post-Phase 1 re-check**: No violations. Single file, single binary, pure Go dependency. No architectural complexity introduced.

## Project Structure

### Documentation (this feature)

```text
specs/001-cross-browser-automation/
├── spec.md
├── plan.md                          # This file
├── research.md                      # Phase 0: library selection rationale
├── data-model.md                    # Phase 1: entity changes
├── quickstart.md                    # Phase 1: build/run guide
├── contracts/
│   └── internal-interfaces.md       # Phase 1: function signature changes
└── checklists/
    └── requirements.md              # Spec quality checklist
```

### Source Code (repository root)

```text
.
├── main.go          # Single file — all changes here
├── go.mod           # Add chromedp, remove gorilla/websocket
├── go.sum           # Updated
├── Makefile         # No changes
└── version.txt      # No changes
```

**Structure Decision**: This project is a single-file Go CLI. No new files or directories are introduced. All code changes happen in `main.go`, with dependency changes in `go.mod`/`go.sum`.

## Implementation Approach

### Step 1: Add chromedp dependency, remove gorilla/websocket

```bash
go get github.com/chromedp/chromedp
go mod tidy  # removes gorilla/websocket after code changes
```

### Step 2: Add Edge paths to browser detection

Update `GetChromePath()` (rename to `GetBrowserPath()`):

| Platform | Current paths | Added paths |
|----------|--------------|-------------|
| macOS | Chrome, Chromium | **Microsoft Edge** |
| Linux | google-chrome, chromium, chromium-browser, snap chromium | **microsoft-edge, microsoft-edge-stable** |
| Windows | Chrome (3 paths) | **Edge (2 paths)** |

Priority order within each platform: Chrome paths first, then Edge, then Chromium.

Return actionable error when no browser found (list supported browsers by platform).

### Step 3: Replace ChromeDebugger with chromedp

**Remove**: `ChromeDebugger` struct, `CDPMessage` struct, `NewChromeDebugger`, `LaunchChrome`, `Connect`, `readMessages`, `handleMessage`, `fetchPostData`, `SendCommand`, `EnableNetworkMonitoring`, `NavigateToURL`, `GetSAMLResponse`, `WaitForSAML`, `Close`, `KillExistingChromeProcesses`.

**Add**: `runBrowserSession(browserPath, url string, timeout time.Duration) (string, error)` that:

1. Creates `ExecAllocator` with:
   - `chromedp.ExecPath(browserPath)` — uses detected browser
   - `chromedp.Flag("headless", false)` — visible window
   - `chromedp.NoFirstRun`, `chromedp.NoDefaultBrowserCheck`
2. Creates `chromedp.NewContext`
3. Sets up `chromedp.ListenTarget` callback for `*network.EventRequestWillBeSent`:
   - Matches POST to `signin.aws.amazon.com`
   - Extracts SAML from `PostDataEntries` or falls back to `network.GetRequestPostData`
   - Sends result on `samlCh` channel
4. Sets up `chromedp.ListenBrowser` for `*target.EventTargetDestroyed` (browser close detection)
5. Runs `network.Enable().WithMaxPostDataSize(1 << 20)` + `chromedp.Navigate(url)`
6. Waits on `samlCh`, timeout, or context cancellation (browser closed)
7. Returns SAML response or descriptive error

**Extract**: `extractSAMLFromPostData` becomes a standalone function (currently a method on `ChromeDebugger`).

### Step 4: Update main() orchestration

Replace the `ChromeDebugger` usage block in `main()` with a single call to `runBrowserSession()`. The surrounding code (CLI parsing, profile selection, SAML parsing, STS call, credential writing) is untouched.

**Before** (main.go lines 716-783):
```
KillExistingChromeProcesses(debugPort)
debugger := NewChromeDebugger(debugPort)
debugger.LaunchChrome(azureURL)
debugger.Connect()
debugger.EnableNetworkMonitoring()
debugger.SendCommand("Page.enable", nil)
debugger.NavigateToURL(azureURL)
samlResponse, err := debugger.WaitForSAML(5 * time.Minute)
```

**After**:
```
browserPath := GetBrowserPath()  // or error with actionable message
samlResponse, err := runBrowserSession(browserPath, azureURL, 5*time.Minute)
```

### Step 5: Improve error messages

| Scenario | Current message | New message |
|----------|----------------|-------------|
| No browser found | "Chrome executable not found" | "No supported browser found. Install one of: Google Chrome, Microsoft Edge, or Chromium.\nSearched paths: [platform-specific list]" |
| Browser closed mid-auth | Hangs or "connection closed" | "Browser was closed before authentication completed. Please run the tool again and complete the Azure AD sign-in." |
| No display (Linux) | Chrome fails silently | "Cannot open browser: no display environment detected. This tool requires a graphical desktop (X11/Wayland)." |

### Step 6: Remove gorilla/websocket

After all code changes, `go mod tidy` removes the unused `gorilla/websocket` dependency.

## Complexity Tracking

No constitution violations. No complexity justifications needed.

## Risk Assessment

| Risk | Likelihood | Mitigation |
|------|-----------|------------|
| chromedp `ListenTarget` callback misses SAML POST | Low | Use `WithMaxPostDataSize(1<<20)` + fallback to `GetRequestPostData`. Same CDP events as current code. |
| Edge binary not found on some Windows installations | Low | Include both `Program Files` and `Program Files (x86)` paths. Same pattern as current Chrome detection. |
| chromedp creates visible window that's behind other windows | Medium | Same issue as current code. User instructions remain: "Check your dock/taskbar for the browser icon." |
| chromedp version incompatibility with older Chrome versions | Low | chromedp supports Chrome 64+. All current Chromium-family browsers are well above this. |
