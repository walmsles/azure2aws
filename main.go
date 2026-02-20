package main

import (
	"context"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/target"
	"github.com/chromedp/chromedp"
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



// GetBrowserPath finds browser executable path (Chrome > Edge > Chromium)
func GetBrowserPath() (string, error) {
	paths := []string{}

	switch runtime.GOOS {
	case "darwin":
		paths = []string{
			// Chrome first
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			// Edge second
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			// Chromium last
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		}
	case "windows":
		paths = []string{
			// Chrome first
			"C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe",
			"C:\\Program Files (x86)\\Google\\Chrome\\Application\\chrome.exe",
			os.Getenv("LOCALAPPDATA") + "\\Google\\Chrome\\Application\\chrome.exe",
			// Edge second
			"C:\\Program Files (x86)\\Microsoft\\Edge\\Application\\msedge.exe",
			"C:\\Program Files\\Microsoft\\Edge\\Application\\msedge.exe",
			os.Getenv("LOCALAPPDATA") + "\\Microsoft\\Edge\\Application\\msedge.exe",
		}
	case "linux":
		paths = []string{
			// Chrome first
			"/usr/bin/google-chrome",
			// Edge second
			"/usr/bin/microsoft-edge",
			"/usr/bin/microsoft-edge-stable",
			// Chromium last
			"/usr/bin/chromium",
			"/usr/bin/chromium-browser",
			"/snap/bin/chromium",
		}
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	// Build error message with platform-specific browser list
	var browserList string
	switch runtime.GOOS {
	case "darwin":
		browserList = "Google Chrome, Microsoft Edge, or Chromium"
	case "windows":
		browserList = "Google Chrome, Microsoft Edge"
	case "linux":
		browserList = "Google Chrome, Microsoft Edge, or Chromium"
	default:
		browserList = "a Chromium-based browser"
	}

	searchedPaths := strings.Join(paths, "\n  ")
	return "", fmt.Errorf("no supported browser found. Install one of: %s.\nSearched paths:\n  %s", browserList, searchedPaths)
}



// readMessages reads messages from Chrome DevTools Protocol

// handleMessage processes incoming CDP messages

// fetchPostData attempts to fetch POST data for a request

// SendCommand sends a command to Chrome DevTools Protocol

// EnableNetworkMonitoring enables ONLY passive network monitoring

// NavigateToURL navigates to the specified URL

// extractSAMLFromPostData extracts SAML response from POST data
func extractSAMLFromPostData(postData string) string {
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

// runBrowserSession launches browser with chromedp and captures the SAML response
func runBrowserSession(browserPath, url string, timeout time.Duration) (string, error) {
	// Create channel for SAML response
	samlCh := make(chan string, 1)

	// Create allocator with the specified browser
	allocCtx, allocCancel := chromedp.NewExecAllocator(
		context.Background(),
		chromedp.ExecPath(browserPath),
		chromedp.Flag("headless", false),
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
	)
	defer allocCancel()

	// Create browser context
	ctx, cancel := chromedp.NewContext(allocCtx)
	defer cancel()

	// Set up network listener for SAML capture
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		switch ev := ev.(type) {
		case *network.EventRequestWillBeSent:
			// Check if this is a POST to signin.aws.amazon.com
			if ev.Request.Method == "POST" && strings.Contains(ev.Request.URL, "signin.aws.amazon.com") {
				log.Println("✓ Detected POST to signin.aws.amazon.com")

				// Check for inline POST data first
				if ev.Request.HasPostData && ev.Request.PostDataEntries != nil && len(ev.Request.PostDataEntries) > 0 {
					// Concatenate all post data entries
					var postData strings.Builder
					for _, entry := range ev.Request.PostDataEntries {
						postData.WriteString(entry.Bytes)
					}

					if saml := extractSAMLFromPostData(postData.String()); saml != "" {
						select {
						case samlCh <- saml:
							log.Println("✓ SAML captured from inline POST data!")
						default:
						}
						return
					}
				}

				// Fall back to fetching POST data separately
				go func() {
					// Need to get the post data using the request ID
					var postData string
					err := chromedp.Run(ctx, chromedp.ActionFunc(func(ctx context.Context) error {
						data, err := network.GetRequestPostData(ev.RequestID).Do(cdp.WithExecutor(ctx, chromedp.FromContext(ctx).Target))
						if err == nil {
							postData = data
						}
						return err
					}))

					if err == nil && postData != "" {
						if saml := extractSAMLFromPostData(postData); saml != "" {
							select {
							case samlCh <- saml:
								log.Println("✓ SAML captured from fetched POST data!")
							default:
							}
						}
					}
				}()
			}
		}
	})

	// Set up browser event listener to detect when browser is closed
	chromedp.ListenBrowser(ctx, func(ev interface{}) {
		if _, ok := ev.(*target.EventTargetDestroyed); ok {
			// Browser window was closed
			cancel()
		}
	})

	// Enable network monitoring and navigate to URL
	log.Printf("Navigating to: %s", url)
	log.Println("Please complete the authentication in the browser window...")

	err := chromedp.Run(ctx,
		network.Enable().WithMaxPostDataSize(1 << 20), // 1MB max post data
		chromedp.Navigate(url),
	)
	if err != nil {
		return "", fmt.Errorf("failed to navigate to URL: %w", err)
	}

	// Wait for SAML response, timeout, or context cancellation
	select {
	case saml := <-samlCh:
		return saml, nil
	case <-time.After(timeout):
		return "", fmt.Errorf("timeout waiting for SAML response after %v", timeout)
	case <-ctx.Done():
		return "", fmt.Errorf("browser was closed before authentication completed. Please run the tool again and complete the Azure AD sign-in")
	}
}

// GetSAMLResponse returns the captured SAML response

// WaitForSAML waits for SAML response with timeout

// Close closes the Chrome instance and connection

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

	fmt.Println("═══════════════════════════════════════════════════════")
	fmt.Println("   AWS SAML Authentication (Chrome CDP)")
	fmt.Println("═══════════════════════════════════════════════════════")
	fmt.Printf("\n📁 Profile: %s\n", profileName)
	fmt.Printf("🌐 Azure URL: %s\n", azureURL)

	// Check for display on Linux
	if runtime.GOOS == "linux" {
		if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
			log.Fatal("❌ Cannot open browser: no display environment detected. This tool requires a graphical desktop (X11 or Wayland).")
		}
	}

	// Get browser path
	browserPath, err := GetBrowserPath()
	if err != nil {
		log.Fatalf("❌ %v", err)
	}

	// Launch browser and capture SAML
	fmt.Println("\n🚀 Launching browser...")
	fmt.Println("\n" + strings.Repeat("─", 55))
	fmt.Println("  📌 IMPORTANT: Check your dock for browser icon")
	fmt.Println("  Click the browser icon to bring window to front")
	fmt.Println("  Then sign in to Azure AD normally")
	fmt.Println(strings.Repeat("─", 55))

	fmt.Println("\n⏳ Waiting for SAML response... (timeout: 5 minutes)")

	samlResponse, err := runBrowserSession(browserPath, azureURL, 5*time.Minute)
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
