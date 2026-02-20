# Internal Interface Contracts

**Date**: 2026-02-19
**Feature**: 001-cross-browser-automation

This is a CLI tool with no external API. Contracts describe internal function signatures that change.

## Functions Removed (replaced by chromedp)

```go
// All of these are replaced by chromedp library calls:
func NewChromeDebugger(debugPort int) *ChromeDebugger
func (cd *ChromeDebugger) LaunchChrome(startURL string) error
func (cd *ChromeDebugger) Connect() error
func (cd *ChromeDebugger) readMessages()
func (cd *ChromeDebugger) handleMessage(msg map[string]interface{})
func (cd *ChromeDebugger) fetchPostData(requestID string)
func (cd *ChromeDebugger) SendCommand(method string, params map[string]interface{}) error
func (cd *ChromeDebugger) EnableNetworkMonitoring() error
func (cd *ChromeDebugger) NavigateToURL(url string) error
func (cd *ChromeDebugger) extractSAMLFromPostData(postData string) string
func (cd *ChromeDebugger) GetSAMLResponse() string
func (cd *ChromeDebugger) WaitForSAML(timeout time.Duration) (string, error)
func (cd *ChromeDebugger) Close()
func KillExistingChromeProcesses(debugPort int)
```

## Functions Modified

```go
// GetChromePath — add Edge paths, rename to reflect broader scope
// Before:
func GetChromePath() string

// After:
func GetBrowserPath() string
// Returns first found path in priority order: Chrome > Edge > Chromium
// Returns ("", error) with actionable message when no browser found
```

## Functions Unchanged

```go
func ParseSAMLResponse(samlResponseB64 string) ([]string, error)
func AssumeRoleWithSAML(samlAssertion, roleArn, principalArn string) (*AWSCredentials, error)
func WriteAWSCredentials(creds *AWSCredentials, profileName string) error
func promptRoleSelection(roles []string) (string, error)
func getConfigDir() string
func saveLastURL(url string)
func loadLastURL() string
func getExistingProfiles() []string
func promptProfileSelection() string
```

## New Functions

```go
// runBrowserSession — replaces ChromeDebugger usage in main()
// Launches browser via chromedp, sets up passive network listener,
// navigates to URL, waits for SAML response or timeout/close
func runBrowserSession(browserPath, url string, timeout time.Duration) (string, error)
// Returns captured SAML response or error

// extractSAMLFromPostData — extracted as standalone (currently a method on ChromeDebugger)
func extractSAMLFromPostData(postData string) string
```

## Structs Removed

```go
// Replaced by chromedp context management:
type ChromeDebugger struct { ... }
type CDPMessage struct { ... }
```

## Structs Unchanged

```go
type SAMLResponse struct { ... }
type Assertion struct { ... }
type AttributeStatement struct { ... }
type Attribute struct { ... }
type AttributeValue struct { ... }
type AWSCredentials struct { ... }
```
