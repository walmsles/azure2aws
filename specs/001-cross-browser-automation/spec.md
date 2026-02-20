# Feature Specification: Replace Manual CDP with Standard Browser Automation Library

**Feature Branch**: `001-cross-browser-automation`
**Created**: 2026-02-19
**Status**: Draft
**Input**: User description: "want to use a standard library rather than the debug manual mechanism we have — good Go automation libraries for browser automation, chrome family + Edge"

## Context

The tool already works on macOS, Linux, and Windows with Chrome and Chromium. The CLI interface, SAML capture, credential writing, profile selection, URL persistence, and multi-role selection all function correctly. This feature replaces the ~400 lines of hand-rolled CDP/WebSocket code with a standard Go browser automation library, adds Edge to the browser detection list, and improves error messages.

**What is NOT changing**: CLI arguments, credential output format, profile selection, URL persistence, SAML parsing, AWS STS integration, multi-role selection, timeout behavior.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Standard Library Replaces Manual CDP Code (Priority: P1)

As a maintainer, I want the raw WebSocket/CDP protocol code replaced with a standard Go browser automation library, so the browser lifecycle, event handling, and network monitoring are managed by a well-maintained library rather than custom code.

**Why this priority**: The current `ChromeDebugger` struct (~400 lines) manually manages WebSocket connections, CDP JSON message parsing, request ID tracking, and background goroutines for message reading. This is fragile, hard to extend, and duplicates what established libraries already provide.

**Independent Test**: Can be tested by running `azure2aws <url>` on each platform and verifying the full authentication flow still works end-to-end with the new library — same behavior, fewer custom lines of code.

**Acceptance Scenarios**:

1. **Given** a user on any supported platform with Chrome installed, **When** they run `azure2aws <azure-url>`, **Then** the authentication flow completes identically to the current version — browser opens, user signs in, SAML is captured, credentials are written.
2. **Given** the updated codebase, **When** compared to the current implementation, **Then** the `ChromeDebugger` struct, `readMessages`, `handleMessage`, `fetchPostData`, raw WebSocket dialing, and manual CDP JSON parsing are all replaced by library calls.
3. **Given** the `gorilla/websocket` dependency, **When** the migration is complete, **Then** it is no longer required (replaced by the library's internal transport).

---

### User Story 2 - Edge Browser Support (Priority: P2)

As a Windows user in a Microsoft enterprise environment where Edge is the default browser, I want azure2aws to detect and use Edge automatically, so I don't need to install Chrome separately.

**Why this priority**: Users of an Azure AD → AWS tool are likely in Microsoft-heavy enterprises where Edge is pushed via group policy and may be the only Chromium-family browser available.

**Independent Test**: Can be tested by running `azure2aws` on a Windows machine with only Edge installed and verifying the full flow works.

**Acceptance Scenarios**:

1. **Given** a Windows machine with Edge but no Chrome or Chromium, **When** the user runs `azure2aws <azure-url>`, **Then** the tool detects Edge and completes the authentication flow.
2. **Given** a machine with both Chrome and Edge, **When** the user runs `azure2aws`, **Then** the tool uses Chrome (priority: Chrome > Edge > Chromium).
3. **Given** Edge on macOS or Linux, **When** the user runs `azure2aws`, **Then** Edge is detected if Chrome and Chromium are absent.

---

### User Story 3 - Improved Error Messages (Priority: P3)

As a user who encounters a problem, I want clear error messages that tell me what went wrong and what to do about it, instead of generic failures.

**Why this priority**: The current code returns bare messages like "Chrome executable not found" with no guidance. Users on Windows with only Edge, or users on headless servers, get no help diagnosing the issue.

**Independent Test**: Can be tested by running `azure2aws` in various failure conditions and verifying the error output is actionable.

**Acceptance Scenarios**:

1. **Given** a machine with no supported browser, **When** the user runs `azure2aws`, **Then** the error message lists which browsers are supported and suggests installing one.
2. **Given** a headless Linux server with no display, **When** the user runs `azure2aws`, **Then** the error message indicates a graphical environment is required.
3. **Given** the browser closes mid-authentication, **When** the tool detects the disconnection, **Then** it displays a message indicating the browser was closed before authentication completed.

---

### Edge Cases

- What happens when the user closes the browser window before completing sign-in? The tool should detect the closed connection and exit with an actionable message.
- What happens when the browser automation library cannot connect to the launched browser? The tool should provide diagnostics (e.g., port conflict, process failed to start).
- What happens on headless Linux servers with no display? The tool should detect this and inform the user.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST replace the current hand-rolled WebSocket/CDP implementation (`ChromeDebugger` struct, `readMessages`, `handleMessage`, `fetchPostData`, `SendCommand`, `Connect`) with a standard Go browser automation library.
- **FR-002**: System MUST detect Edge alongside Chrome and Chromium on all platforms (macOS, Linux, Windows), using priority order: Chrome > Edge > Chromium.
- **FR-003**: System MUST provide actionable error messages when: no supported browser is found (list supported browsers), browser fails to launch, browser closes mid-session, or no graphical display is available.
- **FR-004**: System MUST preserve all existing behavior: CLI interface, SAML capture, credential writing, profile selection, URL persistence, multi-role selection, and 5-minute timeout.
- **FR-005**: System MUST passively monitor network traffic to capture the SAML response without modifying or intercepting the authentication flow.

### Key Entities

- **Browser Session**: A browser window managed by the automation library. Replaces the current manual process launch + WebSocket connection.
- **SAML Response**: Unchanged — base64-encoded SAML assertion captured from POST to `signin.aws.amazon.com`.
- **AWS Credentials**: Unchanged — temporary credentials from STS AssumeRoleWithSAML.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: The full authentication flow works identically on macOS, Linux, and Windows after the library swap.
- **SC-002**: Edge is detected and usable on all three platforms when Chrome and Chromium are absent.
- **SC-003**: The `ChromeDebugger` struct and associated manual CDP code (~400 lines) are replaced by library calls, reducing custom browser protocol code by at least 80%.
- **SC-004**: Error messages in browser-not-found, browser-closed, and no-display scenarios include actionable guidance.
- **SC-005**: The `gorilla/websocket` dependency is removed from `go.mod`.

## Clarifications

### Session 2026-02-19

- Q: Are all browsers (Chrome, Firefox, Edge, Safari, Chromium) required? → A: Chromium-family only (Chrome, Edge, Chromium). Safari/Firefox deferred.
- Q: Browser priority when multiple installed? → A: Chrome > Edge > Chromium, auto-detected.
- Q: Safari/WebKit approach? → A: Deferred. No Go library drives real Safari. Chromium-family via pure-Go CDP library for now.
- Spec scoped down from broad "cross-browser" feature to actual delta: library swap + Edge detection + better error messages.

## Assumptions

- The existing CLI interface, SAML parsing, AWS STS integration, credential writing, profile selection, URL persistence, and multi-role selection are all working correctly and will not be modified.
- The Go module name (`acn2aws`) and binary name (`azure2aws`) remain the same.
- The standard automation library will handle browser process lifecycle and CDP communication, replacing the `ChromeDebugger` struct.
- Edge uses the same CDP protocol as Chrome/Chromium, so no additional protocol handling is needed.
