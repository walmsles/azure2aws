# azure2aws

Authenticate to AWS using Azure AD SAML via Chrome automation.

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

1. Launches Chrome with debugging enabled
2. Navigates to your Azure AD sign-in URL
3. You sign in normally in the browser
4. Captures the SAML response automatically
5. Extracts AWS credentials and saves to `~/.aws/credentials`

## Requirements

- macOS or Linux
- Chrome browser
- Go 1.19+ (for building from source)

## Test your credentials

```bash
aws sts get-caller-identity --profile your-profile
```
