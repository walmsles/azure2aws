# Tasks: Replace Manual CDP with Standard Browser Automation Library

**Input**: Design documents from `/specs/001-cross-browser-automation/`
**Prerequisites**: plan.md (required), spec.md (required), research.md, data-model.md, contracts/

**Tests**: No test tasks included — no test suite exists and none requested.

**Organization**: Tasks grouped by user story. All changes are in `main.go` and `go.mod` (single-file CLI project).

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different functions/sections, no dependencies)
- **[Story]**: US1 = Library swap, US2 = Edge detection, US3 = Error messages
- All file paths relative to repository root

---

## Phase 1: Setup

**Purpose**: Add the chromedp dependency before any code changes

- [X] T001 Add chromedp dependency to go.mod — run `go get github.com/chromedp/chromedp` from repository root

---

## Phase 2: Foundational

**Purpose**: Prepare shared code that multiple user stories depend on

- [X] T002 Extract `extractSAMLFromPostData` from a `ChromeDebugger` method to a standalone package-level function in main.go — change `func (cd *ChromeDebugger) extractSAMLFromPostData(postData string) string` to `func extractSAMLFromPostData(postData string) string` and update the one call site in `handleMessage` (line ~317) to call it as `extractSAMLFromPostData(postData)` instead of `cd.extractSAMLFromPostData(postData)`

**Checkpoint**: Foundation ready — user story implementation can begin

---

## Phase 3: User Story 1 — Standard Library Replaces Manual CDP (Priority: P1) — MVP

**Goal**: Replace the entire `ChromeDebugger` struct and ~400 lines of hand-rolled CDP/WebSocket code with chromedp library calls. The full SAML authentication flow must work identically.

**Independent Test**: Run `azure2aws <url>` with Chrome installed. Browser opens, user signs in to Azure AD, SAML is captured, AWS credentials are written to `~/.aws/credentials`. Behavior identical to current version.

### Implementation for User Story 1

- [X] T003 [US1] Implement `runBrowserSession(browserPath, url string, timeout time.Duration) (string, error)` function in main.go — this is the core replacement for all ChromeDebugger functionality. Must: (1) create `chromedp.NewExecAllocator` with `chromedp.ExecPath(browserPath)`, `chromedp.Flag("headless", false)`, `chromedp.NoFirstRun`, `chromedp.NoDefaultBrowserCheck`; (2) create `chromedp.NewContext`; (3) set up `chromedp.ListenTarget` callback that type-switches on `*network.EventRequestWillBeSent`, checks for POST to `signin.aws.amazon.com`, extracts SAML from `PostDataEntries` or falls back to `network.GetRequestPostData(requestID).Do(cdp.WithExecutor(ctx, c.Target))` in a goroutine, sends result on a `samlCh chan string`; (4) set up `chromedp.ListenBrowser` for `*target.EventTargetDestroyed` to detect browser close; (5) run `network.Enable().WithMaxPostDataSize(1 << 20)` then `chromedp.Navigate(url)` via `chromedp.Run`; (6) select on `samlCh`, `time.After(timeout)`, or `ctx.Done()` and return SAML string or error. New imports needed: `github.com/chromedp/chromedp`, `github.com/chromedp/cdproto/cdp`, `github.com/chromedp/cdproto/network`, `github.com/chromedp/cdproto/target`

- [X] T004 [US1] Update `main()` function in main.go — replace the ChromeDebugger usage block (currently lines ~716–783: `KillExistingChromeProcesses`, `NewChromeDebugger`, `LaunchChrome`, `Connect`, `EnableNetworkMonitoring`, `SendCommand("Page.enable")`, `NavigateToURL`, `WaitForSAML`) with two calls: `browserPath := GetChromePath()` (check for empty string, fatal if not found) then `samlResponse, err := runBrowserSession(browserPath, azureURL, 5*time.Minute)`. Remove the `debugPort` variable, `defer debugger.Close()`, and all status print lines between browser launch and SAML capture (replace with simpler messages). Keep all code before (CLI parsing, profile selection) and after (SAML parsing, STS call, credential writing) unchanged.

- [X] T005 [US1] Remove all ChromeDebugger code from main.go — delete the following: `ChromeDebugger` struct (lines ~60–71), `CDPMessage` struct (lines ~74–78), `NewChromeDebugger` function, `KillExistingChromeProcesses` function, `LaunchChrome` method, `Connect` method, `readMessages` method, `handleMessage` method, `fetchPostData` method, `SendCommand` method, `EnableNetworkMonitoring` method, `NavigateToURL` method, `GetSAMLResponse` method, `WaitForSAML` method, `Close` method. This removes ~350 lines. The standalone `extractSAMLFromPostData` (from T002) and `GetChromePath` must be preserved.

- [X] T006 [US1] Remove gorilla/websocket dependency — delete `"github.com/gorilla/websocket"` from the import block in main.go, also remove any now-unused imports (`"io"`, `"net/http"`, `"sync"` — verify each is still used before removing). Run `go mod tidy` to clean up go.mod and go.sum. Verify `gorilla/websocket` no longer appears in go.mod.

**Checkpoint**: US1 complete. `azure2aws` should build and work identically to the current version using chromedp instead of raw CDP. The `gorilla/websocket` dependency is gone. Run `make build` to verify.

---

## Phase 4: User Story 2 — Edge Browser Support (Priority: P2)

**Goal**: Detect Microsoft Edge alongside Chrome and Chromium on all platforms, with priority order Chrome > Edge > Chromium.

**Independent Test**: On a machine with Edge installed (but Chrome removed), run `azure2aws <url>` and verify Edge is launched and the full flow works.

### Implementation for User Story 2

- [X] T007 [US2] Rename `GetChromePath` to `GetBrowserPath` and add Edge executable paths in main.go — rename the function and update its call site in `main()`. Add Edge paths in priority position (after Chrome, before Chromium) for each platform: macOS add `/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge`; Linux add `/usr/bin/microsoft-edge` and `/usr/bin/microsoft-edge-stable`; Windows add `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe` and `C:\Program Files\Microsoft\Edge\Application\msedge.exe` (also add `os.Getenv("LOCALAPPDATA") + "\\Microsoft\\Edge\\Application\\msedge.exe"`). Final path order per platform must be: Chrome paths → Edge paths → Chromium paths.

**Checkpoint**: US2 complete. Edge is detected on all platforms. Priority order enforced. Run `make build` to verify.

---

## Phase 5: User Story 3 — Improved Error Messages (Priority: P3)

**Goal**: Replace generic error messages with actionable guidance for common failure scenarios.

**Independent Test**: Run `azure2aws` with no supported browser installed — verify the error lists supported browsers. Run on headless Linux — verify it mentions needing a display.

### Implementation for User Story 3

- [X] T008 [P] [US3] Update `GetBrowserPath` return signature to `(string, error)` and return actionable error in main.go — change return type from `string` to `(string, error)`. When no browser found, return an error with platform-specific message listing supported browsers and the paths that were searched. Example for macOS: `"No supported browser found. Install one of: Google Chrome, Microsoft Edge, or Chromium.\nSearched: /Applications/Google Chrome.app/..., /Applications/Microsoft Edge.app/..., /Applications/Chromium.app/..."`. Update the call site in `main()` to handle the error with `log.Fatalf`.

- [X] T009 [P] [US3] Add browser-closed-mid-auth error message in `runBrowserSession()` in main.go — when the `ctx.Done()` case fires in the select loop (meaning browser was closed or crashed before SAML was captured), return the error: `"Browser was closed before authentication completed. Please run the tool again and complete the Azure AD sign-in."` instead of a generic context cancellation message.

- [X] T010 [P] [US3] Add no-display detection for Linux before browser launch in main.go — in `main()` or at the start of `runBrowserSession()`, check `runtime.GOOS == "linux"` and if both `os.Getenv("DISPLAY")` and `os.Getenv("WAYLAND_DISPLAY")` are empty, return/fatal with: `"Cannot open browser: no display environment detected. This tool requires a graphical desktop (X11 or Wayland)."`. This prevents a confusing chromedp crash on headless Linux servers.

**Checkpoint**: US3 complete. All three error scenarios produce actionable messages.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Purpose**: Final cleanup and build verification

- [X] T011 Verify clean build and remove any remaining unused imports in main.go — run `make build` to confirm compilation succeeds with no errors. Check for unused imports (`io`, `net/http`, `sync`, `fmt` variants) and remove any that are no longer referenced. Run `go vet ./...` to catch any issues.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies — start immediately
- **Foundational (Phase 2)**: Depends on Phase 1 (chromedp in go.mod)
- **US1 (Phase 3)**: Depends on Phase 2 (extractSAMLFromPostData extracted)
- **US2 (Phase 4)**: Depends on Phase 3 (US1 must be complete so GetChromePath call site is stable)
- **US3 (Phase 5)**: Depends on Phase 4 (GetBrowserPath must exist with its final name)
- **Polish (Phase 6)**: Depends on all previous phases

### Task Dependencies Within Phases

- Phase 3: T003 → T004 → T005 → T006 (strictly sequential — build new, wire up, remove old, clean deps)
- Phase 4: T007 (single task)
- Phase 5: T008, T009, T010 are [P] — they touch different functions and can run in parallel

### Parallel Opportunities

- **Phase 5 tasks T008, T009, T010** touch separate functions (`GetBrowserPath`, `runBrowserSession`, and `main`/display check respectively) and can be implemented in parallel

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001)
2. Complete Phase 2: Foundational (T002)
3. Complete Phase 3: User Story 1 (T003–T006)
4. **STOP and VALIDATE**: `make build` succeeds, `azure2aws <url>` works identically to before
5. At this point the main goal (library swap) is done

### Incremental Delivery

1. T001–T006 → Library swap complete → validate with `make build`
2. T007 → Edge detection added → validate on Windows/Edge if available
3. T008–T010 → Error messages improved → validate by testing failure scenarios
4. T011 → Final cleanup → ship

---

## Notes

- All 11 tasks modify the same two files: `main.go` and `go.mod`
- Phase 3 (US1) is the bulk of the work — T003 is the largest single task
- No test tasks included — project has no test suite and none was requested
- The current code leaks temp directories (`os.MkdirTemp` without cleanup) — chromedp fixes this automatically
- `KillExistingChromeProcesses` is removed entirely — chromedp manages its own browser process isolation
