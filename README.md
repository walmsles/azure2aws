# azure2aws

Authenticate to AWS using Azure AD SAML via browser automation.

## Installation

### Homebrew
```bash
brew tap walmsles/tap
brew install azure2aws
```

### Build from source
```bash
make
```

## Usage

```bash
azure2aws <azure-signin-url> [profile-name]
```

**Examples:**
```bash
# Interactive profile selection
azure2aws https://myapps.microsoft.com/signin/AWS/xxxxx

# Specify profile
azure2aws https://myapps.microsoft.com/signin/AWS/xxxxx my-profile
```

## How it works

1. Detects an installed Chromium-based browser (Chrome, Edge, or Chromium)
2. Launches the browser and navigates to your Azure AD sign-in URL
3. You sign in normally in the browser
4. Captures the SAML response automatically
5. Extracts AWS credentials and saves to `~/.aws/credentials`

## Requirements

### System Requirements
- **Operating System**: macOS, Linux, or Windows
- **Browser**: Google Chrome, Microsoft Edge, or Chromium
- **Go**: 1.24+ (for building from source only)

### Browser Support

Priority order when multiple browsers are installed: Chrome > Edge > Chromium.

- ✅ **Google Chrome**: Full support
- ✅ **Microsoft Edge**: Full support
- ✅ **Chromium**: Full support
- ❌ **Firefox**: Not supported (not Chromium-based)
- ❌ **Safari**: Not supported (not Chromium-based)

### Constraints
- A supported browser must be installed in a standard location
- Requires a graphical desktop (X11 or Wayland on Linux)
- Requires network access to Azure AD and AWS endpoints

## Test your credentials

```bash
aws sts get-caller-identity --profile your-profile
```
