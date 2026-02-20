<!-- Sync Impact Report
Version change: 0.0.0 → 1.0.0 (initial constitution)
Modified principles: N/A (initial creation)
Added sections: All sections newly created
Removed sections: N/A
Templates requiring updates: ✅ All templates align with initial constitution
Follow-up TODOs: RATIFICATION_DATE needs confirmation
-->

# azure2aws Constitution

## Core Principles

### I. Single Binary Simplicity
The tool MUST remain a single statically-compiled Go binary with zero runtime dependencies. All functionality lives in main.go unless splitting provides measurable user value. No external processes, interpreters, or configuration files required for core operation.

**Rationale**: Users need a tool they can download and immediately use without installation complexity or dependency management.

### II. Browser Agnostic Automation
The tool MUST support all Chromium-based browsers (Chrome, Edge, Chromium, Brave, etc.) using standard CDP libraries. Browser detection follows priority order: Chrome → Edge → Chromium → others. The tool MUST NOT implement custom CDP protocol handling when mature libraries exist.

**Rationale**: Users have different browser preferences and corporate policies. Using standard libraries reduces maintenance burden and improves reliability.

### III. Passive Network Monitoring Only
The tool MUST use passive network observation to capture SAML responses. It MUST NOT intercept, modify, or proxy network traffic. Authentication flow remains entirely user-controlled through the browser UI.

**Rationale**: Security and trust - users need confidence the tool doesn't interfere with their authentication or credentials.

### IV. Actionable Error Messages
Every error message MUST tell the user how to fix the problem. Include what was expected, what was found, and specific steps to resolve. Platform-specific guidance required where behavior differs.

**Rationale**: CLI tools often run in automated contexts where generic errors waste debugging time.

### V. Minimal Configuration Philosophy
The tool MUST work with zero configuration for common cases. Optional configuration (profiles, URL persistence) enhances but never gates core functionality. No config files, environment variables, or setup wizards required for first use.

**Rationale**: Reduces friction for new users and simplifies troubleshooting.

## Architecture Constraints

### Code Organization
- Single file architecture (main.go) until complexity genuinely demands splitting
- No packages/modules unless they can be independently useful
- Platform-specific code isolated to dedicated functions
- Concurrency limited to necessary network monitoring

### Dependency Management
- Prefer standard library over external dependencies
- When external dependencies needed, choose mature, well-maintained libraries
- Document why each dependency exists in CLAUDE.md
- Regular dependency audits to remove unused imports

### Security Boundaries
- Never store credentials longer than necessary
- Clear separation between browser automation and credential handling
- No credential logging even in debug mode
- Temporary browser profiles must be cleaned up

## Development Workflow

### Change Process
1. Document feature in specs/ using SpecKit templates
2. Implementation follows task breakdown in tasks.md
3. Manual testing required for authentication flow
4. Update CLAUDE.md with architectural changes
5. Version bump via Makefile targets

### Testing Philosophy
Given the nature of browser automation and authentication:
- Manual end-to-end testing remains primary validation
- Mock testing of SAML parsing acceptable
- Browser automation cannot be meaningfully unit tested
- Focus testing effort on error conditions and edge cases

### Compatibility Commitments
- Maintain backward compatibility for CLI arguments
- Profile format in ~/.aws/credentials must remain AWS CLI compatible
- Breaking changes require major version bump
- Support current and previous OS versions (macOS, Linux, Windows)

## Governance

### Amendment Process
- Constitution changes require justification in PR description
- Breaking principle changes require migration plan
- Version bumps follow semantic versioning
- All changes must maintain single-binary philosophy

### Compliance Verification
- PR reviews must verify constitution compliance
- Complexity additions must be justified against user value
- Dependencies additions require explicit rationale
- Error messages reviewed for actionability

### Authority
This constitution supersedes all other project documentation. In case of conflicts, constitution principles take precedence. Use CLAUDE.md for implementation guidance that aligns with these principles.

**Version**: 1.0.0 | **Ratified**: TODO(RATIFICATION_DATE) | **Last Amended**: 2025-02-20