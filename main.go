package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/gorilla/websocket"
	"gopkg.in/ini.v1"
)

var version = "dev"

// SAML Response structures
type SAMLResponse struct {
	XMLName   xml.Name  `xml:"Response"`
	Assertion Assertion `xml:"Assertion"`
}

type Assertion struct {
	AttributeStatement AttributeStatement `xml:"AttributeStatement"`
}

type AttributeStatement struct {
	Attributes []Attribute `xml:"Attribute"`
}

type Attribute struct {
	Name            string           `xml:"Name,attr"`
	AttributeValues []AttributeValue `xml:"AttributeValue"`
}

type AttributeValue struct {
	Value string `xml:",chardata"`
}

// AWSCredentials holds temporary AWS credentials
type AWSCredentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Expiration      time.Time
}

// ChromeDebugger manages Chrome DevTools Protocol connection
type ChromeDebugger struct {
	cmd          *exec.Cmd
	wsURL        string
	conn         *websocket.Conn
	debugPort    int
	samlResponse string
	requestID    int
	mu           sync.Mutex
	done         chan struct{}
	requestIDs   map[string]bool
}

// CDPMessage represents Chrome DevTools Protocol message
type CDPMessage struct {
	ID     int                    `json:"id"`
	Method string                 `json:"method"`
	Params map[string]interface{} `json:"params,omitempty"`
}

// NewChromeDebugger creates a new Chrome instance with debugging enabled
func NewChromeDebugger(debugPort int) *ChromeDebugger {
	return &ChromeDebugger{
		debugPort:  debugPort,
		requestID:  1,
		done:       make(chan struct{}),
		requestIDs: make(map[string]bool),
	}
}

// KillExistingChromeProcesses kills any existing Chrome processes with debugging enabled
func KillExistingChromeProcesses(debugPort int) {
	switch runtime.GOOS {
	case "darwin":
		// Kill Chrome processes with our debug port
		exec.Command("pkill", "-f", fmt.Sprintf("remote-debugging-port=%d", debugPort)).Run()
	case "linux":
		exec.Command("pkill", "-f", fmt.Sprintf("remote-debugging-port=%d", debugPort)).Run()
	case "windows":
		exec.Command("taskkill", "/F", "/IM", "chrome.exe").Run()
	}

	// Wait a moment for processes to terminate
	time.Sleep(2 * time.Second)
}

// GetChromePath finds Chrome executable path
func GetChromePath() string {
	paths := []string{}

	switch runtime.GOOS {
	case "darwin":
		paths = []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		}
	case "windows":
		paths = []string{
			"C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe",
			"C:\\Program Files (x86)\\Google\\Chrome\\Application\\chrome.exe",
			os.Getenv("LOCALAPPDATA") + "\\Google\\Chrome\\Application\\chrome.exe",
		}
	case "linux":
		paths = []string{
			"/usr/bin/google-chrome",
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"/snap/bin/chromium",
		}
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return ""
}

// LaunchChrome starts Chrome with remote debugging enabled
func (cd *ChromeDebugger) LaunchChrome(startURL string) error {
	chromePath := GetChromePath()
	if chromePath == "" {
		return fmt.Errorf("Chrome executable not found")
	}

	// Create user data directory
	userDataDir, err := os.MkdirTemp("", "chrome-debug-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}

	args := []string{
		fmt.Sprintf("--remote-debugging-port=%d", cd.debugPort),
		"--user-data-dir=" + userDataDir,
		"--no-first-run",
		"--no-default-browser-check",
		startURL,
	}

	log.Printf("Chrome path: %s", chromePath)
	log.Printf("Chrome args: %v", args)
	log.Printf("User data dir: %s", userDataDir)

	cd.cmd = exec.Command(chromePath, args...)

	if err := cd.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start Chrome: %w", err)
	}

	log.Printf("Chrome process started with PID: %d", cd.cmd.Process.Pid)

	// Wait longer for Chrome to be ready and create page targets
	time.Sleep(5 * time.Second)

	return nil
}

// Connect connects to Chrome DevTools Protocol using Browser target
func (cd *ChromeDebugger) Connect() error {
	// Retry connection with backoff
	maxRetries := 10
	for attempt := 1; attempt <= maxRetries; attempt++ {
		log.Printf("Connection attempt %d/%d...", attempt, maxRetries)

		// Get all targets
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/json", cd.debugPort))
		if err != nil {
			if attempt == maxRetries {
				return fmt.Errorf("failed to connect to Chrome after %d attempts: %w", maxRetries, err)
			}
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			if attempt == maxRetries {
				return fmt.Errorf("failed to read response: %w", err)
			}
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}

		var targets []map[string]interface{}
		if err := json.Unmarshal(body, &targets); err != nil {
			if attempt == maxRetries {
				return fmt.Errorf("failed to parse JSON: %w", err)
			}
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}

		log.Printf("Found %d targets", len(targets))

		// Find the first page target
		var wsURL string
		for _, target := range targets {
			targetType, _ := target["type"].(string)
			targetURL, _ := target["url"].(string)
			log.Printf("Target: type=%s, url=%s", targetType, targetURL)

			if targetType == "page" {
				wsURL, _ = target["webSocketDebuggerUrl"].(string)
				break
			}
		}

		if wsURL == "" {
			if attempt == maxRetries {
				return fmt.Errorf("no page target found after %d attempts", maxRetries)
			}
			log.Printf("No page target found, retrying in %d seconds...", attempt)
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}

		cd.wsURL = wsURL

		// Connect to WebSocket
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			if attempt == maxRetries {
				return fmt.Errorf("failed to connect to WebSocket: %w", err)
			}
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}

		cd.conn = conn

		// Start message reader
		go cd.readMessages()

		log.Printf("Successfully connected to Chrome DevTools Protocol")
		return nil
	}

	return fmt.Errorf("failed to connect after %d attempts", maxRetries)
}

// readMessages reads messages from Chrome DevTools Protocol
func (cd *ChromeDebugger) readMessages() {
	defer close(cd.done)
	for {
		_, message, err := cd.conn.ReadMessage()
		if err != nil {
			return
		}

		var msg map[string]interface{}
		if err := json.Unmarshal(message, &msg); err != nil {
			continue
		}

		cd.handleMessage(msg)
	}
}

// handleMessage processes incoming CDP messages
func (cd *ChromeDebugger) handleMessage(msg map[string]interface{}) {
	method, ok := msg["method"].(string)
	if !ok {
		return
	}

	// Handle network events - look for requests
	if method == "Network.requestWillBeSent" {
		params, ok := msg["params"].(map[string]interface{})
		if !ok {
			return
		}

		requestID, _ := params["requestId"].(string)
		request, ok := params["request"].(map[string]interface{})
		if !ok {
			return
		}

		reqURL, _ := request["url"].(string)
		reqMethod, _ := request["method"].(string)

		// Detect POST to AWS signin
		if reqMethod == "POST" && strings.Contains(reqURL, "signin.aws.amazon.com") {
			log.Printf("🔍 Detected POST to AWS: %s", reqURL)

			// Store request ID so we can fetch the body
			cd.mu.Lock()
			cd.requestIDs[requestID] = true
			cd.mu.Unlock()

			// Try to get POST data if available directly
			if postData, ok := request["postData"].(string); ok && postData != "" {
				log.Println("✓ Found POST data in request")
				saml := cd.extractSAMLFromPostData(postData)
				if saml != "" {
					cd.mu.Lock()
					cd.samlResponse = saml
					cd.mu.Unlock()
					log.Println("✓ SAML captured from POST data!")
				}
			} else {
				// Request the post data explicitly
				log.Println("📡 POST data not in request, trying to fetch...")
				go cd.fetchPostData(requestID)
			}
		}
	}

	// Also check for response loading finished (backup method)
	if method == "Network.loadingFinished" {
		params, ok := msg["params"].(map[string]interface{})
		if !ok {
			return
		}

		requestID, _ := params["requestId"].(string)

		cd.mu.Lock()
		shouldFetch := cd.requestIDs[requestID]
		cd.mu.Unlock()

		if shouldFetch && cd.GetSAMLResponse() == "" {
			go cd.fetchPostData(requestID)
		}
	}
}

// fetchPostData attempts to fetch POST data for a request
func (cd *ChromeDebugger) fetchPostData(requestID string) {
	// Send command to get post data
	cd.mu.Lock()
	id := cd.requestID
	cd.requestID++
	cd.mu.Unlock()

	cmd := map[string]interface{}{
		"id":     id,
		"method": "Network.getRequestPostData",
		"params": map[string]interface{}{
			"requestId": requestID,
		},
	}

	if err := cd.conn.WriteJSON(cmd); err != nil {
		log.Printf("Failed to request post data: %v", err)
		return
	}

	// Note: Response will come back in readMessages as a reply with matching id
	// We'll need to handle it there, but for simplicity, we're relying on the direct method
}

// SendCommand sends a command to Chrome DevTools Protocol
func (cd *ChromeDebugger) SendCommand(method string, params map[string]interface{}) error {
	cd.mu.Lock()
	id := cd.requestID
	cd.requestID++
	cd.mu.Unlock()

	msg := CDPMessage{
		ID:     id,
		Method: method,
		Params: params,
	}

	return cd.conn.WriteJSON(msg)
}

// EnableNetworkMonitoring enables ONLY passive network monitoring
func (cd *ChromeDebugger) EnableNetworkMonitoring() error {
	// ONLY enable Network - no interception, no fetch, no blocking
	return cd.SendCommand("Network.enable", nil)
}

// NavigateToURL navigates to the specified URL
func (cd *ChromeDebugger) NavigateToURL(url string) error {
	params := map[string]interface{}{
		"url": url,
	}
	return cd.SendCommand("Page.navigate", params)
}

// extractSAMLFromPostData extracts SAML response from POST data
func (cd *ChromeDebugger) extractSAMLFromPostData(postData string) string {
	// Parse URL-encoded form data
	values, err := url.ParseQuery(postData)
	if err != nil {
		// Try manual parsing
		parts := strings.Split(postData, "&")
		for _, part := range parts {
			if strings.HasPrefix(part, "SAMLResponse=") {
				saml := strings.TrimPrefix(part, "SAMLResponse=")
				decoded, err := url.QueryUnescape(saml)
				if err == nil {
					return decoded
				}
				return saml
			}
		}
		return ""
	}

	return values.Get("SAMLResponse")
}

// GetSAMLResponse returns the captured SAML response
func (cd *ChromeDebugger) GetSAMLResponse() string {
	cd.mu.Lock()
	defer cd.mu.Unlock()
	return cd.samlResponse
}

// WaitForSAML waits for SAML response with timeout
func (cd *ChromeDebugger) WaitForSAML(timeout time.Duration) (string, error) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	deadline := time.Now().Add(timeout)

	for {
		select {
		case <-cd.done:
			return "", fmt.Errorf("connection closed")
		case <-ticker.C:
			saml := cd.GetSAMLResponse()
			if saml != "" {
				return saml, nil
			}
			if time.Now().After(deadline) {
				return "", fmt.Errorf("timeout waiting for SAML response")
			}
		}
	}
}

// Close closes the Chrome instance and connection
func (cd *ChromeDebugger) Close() {
	if cd.conn != nil {
		cd.conn.Close()
	}
	if cd.cmd != nil && cd.cmd.Process != nil {
		cd.cmd.Process.Kill()
		cd.cmd.Wait()
	}
}

// ParseSAMLResponse parses the SAML response and extracts AWS roles
func ParseSAMLResponse(samlResponseB64 string) ([]string, error) {
	decoded, err := base64.StdEncoding.DecodeString(samlResponseB64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode SAML response: %w", err)
	}

	var samlResp SAMLResponse
	if err := xml.Unmarshal(decoded, &samlResp); err != nil {
		return nil, fmt.Errorf("failed to parse SAML XML: %w", err)
	}

	var roles []string
	for _, attr := range samlResp.Assertion.AttributeStatement.Attributes {
		if attr.Name == "https://aws.amazon.com/SAML/Attributes/Role" {
			for _, value := range attr.AttributeValues {
				roles = append(roles, value.Value)
			}
		}
	}

	if len(roles) == 0 {
		return nil, fmt.Errorf("no AWS roles found in SAML response")
	}

	return roles, nil
}

// AssumeRoleWithSAML uses AWS STS to assume a role with SAML assertion
func AssumeRoleWithSAML(samlAssertion, roleArn, principalArn string) (*AWSCredentials, error) {
	ctx := context.Background()

	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion("us-east-1"))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	stsClient := sts.NewFromConfig(cfg)

	input := &sts.AssumeRoleWithSAMLInput{
		RoleArn:       &roleArn,
		PrincipalArn:  &principalArn,
		SAMLAssertion: &samlAssertion,
	}

	result, err := stsClient.AssumeRoleWithSAML(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to assume role: %w", err)
	}

	return &AWSCredentials{
		AccessKeyID:     *result.Credentials.AccessKeyId,
		SecretAccessKey: *result.Credentials.SecretAccessKey,
		SessionToken:    *result.Credentials.SessionToken,
		Expiration:      *result.Credentials.Expiration,
	}, nil
}

// WriteAWSCredentials writes credentials to ~/.aws/credentials
func WriteAWSCredentials(creds *AWSCredentials, profileName string) error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	awsDir := filepath.Join(homeDir, ".aws")
	credsFile := filepath.Join(awsDir, "credentials")

	if err := os.MkdirAll(awsDir, 0700); err != nil {
		return fmt.Errorf("failed to create .aws directory: %w", err)
	}

	cfg, err := ini.Load(credsFile)
	if err != nil {
		cfg = ini.Empty()
	}

	section, err := cfg.NewSection(profileName)
	if err != nil {
		section, _ = cfg.GetSection(profileName)
	}

	section.Key("aws_access_key_id").SetValue(creds.AccessKeyID)
	section.Key("aws_secret_access_key").SetValue(creds.SecretAccessKey)
	section.Key("aws_session_token").SetValue(creds.SessionToken)

	if err := cfg.SaveTo(credsFile); err != nil {
		return fmt.Errorf("failed to save credentials: %w", err)
	}

	return nil
}

func promptRoleSelection(roles []string) (string, error) {
	if len(roles) == 1 {
		return roles[0], nil
	}

	fmt.Println("\n📋 Multiple AWS roles found:")
	for i, role := range roles {
		fmt.Printf("%d. %s\n", i+1, role)
	}

	fmt.Print("\nSelect role number (1-", len(roles), "): ")
	var choice int
	_, err := fmt.Scanf("%d", &choice)
	if err != nil || choice < 1 || choice > len(roles) {
		return "", fmt.Errorf("invalid selection")
	}

	return roles[choice-1], nil
}

// getConfigDir returns the config directory path
func getConfigDir() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".acn2aws")
}

// saveLastURL saves the last used URL to config file
func saveLastURL(url string) {
	configDir := getConfigDir()
	os.MkdirAll(configDir, 0755)

	configFile := filepath.Join(configDir, "config")
	file, err := os.Create(configFile)
	if err != nil {
		return
	}
	defer file.Close()

	fmt.Fprintf(file, "last_url=%s\n", url)
}

// loadLastURL loads the last used URL from config file
func loadLastURL() string {
	configFile := filepath.Join(getConfigDir(), "config")
	data, err := os.ReadFile(configFile)
	if err != nil {
		return ""
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "last_url=") {
			return strings.TrimPrefix(line, "last_url=")
		}
	}
	return ""
}

// getExistingProfiles returns list of existing AWS profiles
func getExistingProfiles() []string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return []string{}
	}

	credsFile := filepath.Join(homeDir, ".aws", "credentials")
	cfg, err := ini.Load(credsFile)
	if err != nil {
		return []string{}
	}

	var profiles []string
	for _, section := range cfg.Sections() {
		if section.Name() != "DEFAULT" {
			profiles = append(profiles, section.Name())
		}
	}
	return profiles
}

// promptProfileSelection prompts user to select or create a profile
func promptProfileSelection() string {
	profiles := getExistingProfiles()

	if len(profiles) > 0 {
		fmt.Println("\n📋 Existing AWS profiles:")
		for i, profile := range profiles {
			fmt.Printf("%d. %s\n", i+1, profile)
		}

		fmt.Printf("\nSelect profile (1-%d) or enter new name: ", len(profiles))
		var input string
		fmt.Scanln(&input)

		// Check if it's a number
		var choice int
		if n, err := fmt.Sscanf(input, "%d", &choice); n == 1 && err == nil {
			if choice >= 1 && choice <= len(profiles) {
				return profiles[choice-1]
			}
		}

		// Treat as new profile name
		return input
	} else {
		fmt.Print("Enter profile name [default]: ")
		var profile string
		fmt.Scanln(&profile)
		if profile == "" {
			return "default"
		}
		return profile
	}
}

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Printf("azure2aws version %s\n", version)
		os.Exit(0)
	}

	var azureURL string

	if len(os.Args) < 2 {
		lastURL := loadLastURL()
		if lastURL != "" {
			fmt.Printf("Enter Azure sign-in URL [%s]: ", lastURL)
		} else {
			fmt.Print("Enter Azure sign-in URL: ")
		}
		fmt.Scanln(&azureURL)
		if azureURL == "" {
			if lastURL != "" {
				azureURL = lastURL
			} else {
				fmt.Println("Usage: aws-saml-auth <azure-signin-url> [profile-name]")
				fmt.Println("Example: aws-saml-auth https://myapps.microsoft.com/signin/AWS/xxxxx my-profile")
				os.Exit(1)
			}
		}
	} else {
		azureURL = os.Args[1]
	}

	// Save the URL for next time
	saveLastURL(azureURL)

	var profileName string
	if len(os.Args) >= 3 {
		profileName = os.Args[2]
	} else {
		profileName = promptProfileSelection()
	}

	debugPort := 9222

	fmt.Println("═══════════════════════════════════════════════════════")
	fmt.Println("   AWS SAML Authentication (Chrome CDP)")
	fmt.Println("═══════════════════════════════════════════════════════")
	fmt.Printf("\n📁 Profile: %s\n", profileName)
	fmt.Printf("🌐 Azure URL: %s\n", azureURL)

	// Kill any existing Chrome processes to avoid conflicts
	fmt.Println("\n🧹 Cleaning up existing Chrome processes...")
	KillExistingChromeProcesses(debugPort)

	// Create Chrome debugger
	debugger := NewChromeDebugger(debugPort)
	defer debugger.Close()

	// Launch Chrome
	fmt.Println("\n🚀 Launching Chrome...")
	if err := debugger.LaunchChrome(azureURL); err != nil {
		log.Fatalf("❌ Failed to launch Chrome: %v", err)
	}

	fmt.Println("✓ Chrome launched")

	// Connect to Chrome DevTools Protocol
	fmt.Println("🔌 Connecting to Chrome DevTools Protocol...")

	if err := debugger.Connect(); err != nil {
		log.Fatalf("❌ Failed to connect: %v", err)
	}

	fmt.Println("✓ Connected")

	// Enable ONLY passive network monitoring
	fmt.Println("📡 Enabling network monitoring (passive only)...")
	if err := debugger.EnableNetworkMonitoring(); err != nil {
		log.Fatalf("❌ Failed to enable network monitoring: %v", err)
	}

	fmt.Println("✓ Network monitoring enabled")

	// Enable Page domain for navigation
	fmt.Println("📄 Enabling Page domain...")
	if err := debugger.SendCommand("Page.enable", nil); err != nil {
		log.Printf("⚠️  Failed to enable Page domain: %v", err)
	}

	// Navigate to the URL explicitly
	fmt.Println("🌐 Navigating to Azure URL...")
	if err := debugger.NavigateToURL(azureURL); err != nil {
		log.Printf("⚠️  Failed to navigate programmatically: %v", err)
	} else {
		fmt.Println("✓ Navigation initiated")
	}

	fmt.Println("\n" + strings.Repeat("─", 55))
	fmt.Println("  📌 IMPORTANT: Check your dock for Chrome icon")
	fmt.Println("  Click the Chrome icon to bring window to front")
	fmt.Println("  Then sign in to Azure AD normally")
	fmt.Println(strings.Repeat("─", 55))

	// Wait for SAML
	fmt.Println("\n⏳ Waiting for SAML response... (timeout: 5 minutes)")

	samlResponse, err := debugger.WaitForSAML(5 * time.Minute)
	if err != nil {
		log.Fatalf("❌ %v", err)
	}

	if samlResponse == "" {
		log.Fatal("❌ No SAML response captured")
	}

	fmt.Println("\n✓ SAML response captured!")

	// Parse SAML
	roles, err := ParseSAMLResponse(samlResponse)
	if err != nil {
		log.Fatalf("❌ Failed to parse SAML: %v", err)
	}

	fmt.Printf("✓ Found %d AWS role(s)\n", len(roles))

	// Select role
	selectedRole, err := promptRoleSelection(roles)
	if err != nil {
		log.Fatalf("❌ Role selection failed: %v", err)
	}

	// Parse ARNs
	parts := strings.Split(selectedRole, ",")
	if len(parts) != 2 {
		log.Fatalf("❌ Invalid role format: %s", selectedRole)
	}

	var roleArn, principalArn string
	if strings.Contains(parts[0], ":role/") {
		roleArn = parts[0]
		principalArn = parts[1]
	} else {
		roleArn = parts[1]
		principalArn = parts[0]
	}

	fmt.Printf("\n🔑 Role ARN: %s\n", roleArn)
	fmt.Printf("🔑 Principal ARN: %s\n", principalArn)

	// Assume role
	fmt.Println("\n⏳ Assuming AWS role...")
	creds, err := AssumeRoleWithSAML(samlResponse, roleArn, principalArn)
	if err != nil {
		log.Fatalf("❌ Failed to assume role: %v", err)
	}

	fmt.Println("✓ Successfully assumed role")
	fmt.Printf("⏰ Expires: %s\n", creds.Expiration.Format(time.RFC3339))

	// Write credentials
	fmt.Printf("\n💾 Writing to profile '%s'...\n", profileName)
	if err := WriteAWSCredentials(creds, profileName); err != nil {
		log.Fatalf("❌ Failed to write credentials: %v", err)
	}

	fmt.Println("✓ Credentials saved")
	fmt.Println("\n═══════════════════════════════════════════════════════")
	fmt.Println("   🎉 Authentication Complete!")
	fmt.Println("═══════════════════════════════════════════════════════")
	fmt.Printf("\n💡 Test: aws sts get-caller-identity --profile %s\n", profileName)
	fmt.Println("\n✓ Chrome will close in 2 seconds...")

	time.Sleep(2 * time.Second)
}
