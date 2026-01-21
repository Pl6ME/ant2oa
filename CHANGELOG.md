# Changelog

All notable changes to this project will be documented in this file.

## [Unreleased]

## [1.1.0] - 2026-01-21

### Added
- **Web UI Configuration Interface**: Added embedded web interface at `/` for easy service configuration
- **Configuration API**: Added `/api/config` (GET/POST) for programmatic configuration management
- **Restart API**: Added `/api/restart` endpoint to restart the service via API
- **Auto-generated .env**: If `.env` file doesn't exist, it is automatically created with default header when saving configuration
- **Missing variable handling**: When updating config, missing required variables are automatically appended to `.env`
- **Single binary deployment**: Web UI files are embedded using `embed` directive - no external files needed

### Changed
- Default port changed from `:0` (random) to `:8080` for consistent access to Web UI
- Configuration can be managed through Web UI, API, or `.env` file

### Fixed
- Improved error handling in `modelsHandler` for HTTP request creation
- Better JSON unmarshaling with fallback to empty string on parse errors
- Added error logging for `.env` file write operations

## [1.0.0] - Initial Release

### Features
- OpenAI-compatible API proxy to Anthropic API format conversion
- Support for messages, completion, and models endpoints
- Rate limiting with configurable RPM
- Health check endpoint
- Systemd service installation for Linux
- Cross-platform support (Windows, Linux, macOS, ARM64)
