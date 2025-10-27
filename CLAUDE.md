# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**mastercrab** is a Go CLI application built for learning Go. It's a Cobra-based CLI tool that interacts with Linear's API and provides file/folder name sanitization utilities.

## Development Commands

### Build and Run
```bash
go run main.go [command]
# or after building:
go build -o mastercrab
./mastercrab [command]
```

### Testing
```bash
go test ./...                    # Run all tests
go test ./cmd/daily -v          # Run specific package tests with verbose output
```

### Dependencies
```bash
go mod tidy                     # Clean up dependencies
go mod download                 # Download dependencies
```

## Architecture

### CLI Structure
- **main.go**: Entry point that calls `cmd.Execute()`
- **cmd/root.go**: Root command configuration with Viper config initialization
- **cmd/daily/**: Daily command for querying Linear API assigned issues
- **cmd/sanitize/**: File/folder name sanitization command (work in progress)

### Configuration
- Uses Viper for configuration management
- Config file: `crab.yaml` (or `$HOME/.crab.yaml`)
- Environment variables prefixed with `CRAB_` are automatically loaded
- Linear API token stored in config: `linear.apiToken`

### Key Components

**Linear API Integration** (`cmd/daily/linear_client.go`):
- Structured GraphQL client for Linear API
- Type definitions for viewer assigned issues queries
- Date filtering support (defaults to last 24 hours with `-P1D`)
- Proper error handling and JSON marshaling/unmarshaling

**Command Pattern**:
- Each subcommand is in its own package under `cmd/`
- Commands are registered in `cmd/root.go` using `rootCmd.AddCommand()`
- Uses Cobra's standard flag and configuration patterns

### Testing Strategy
- Uses `github.com/stretchr/testify` for assertions
- Mock responses stored in `mockResponses/` directory
- Linear client has dedicated test file: `cmd/daily/linear_client_test.go`

## Important Notes

- The Linear API token in `crab.yaml` should be treated as sensitive
- The daily command queries for issues updated in the last 4 days by default (`-P4D`)
- The sanitize command is not fully implemented yet - it has placeholder logic
- Authorization is currently disabled in some parts (see TODO comment in `linear_client.go:69`)