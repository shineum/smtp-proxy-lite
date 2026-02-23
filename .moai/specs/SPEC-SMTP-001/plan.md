---
id: SPEC-SMTP-001
type: plan
spec_ref: SPEC-SMTP-001
---

# Implementation Plan: SMTP Proxy Lite

## Project Structure

```
smtp-proxy-lite/
├── cmd/
│   └── smtp-proxy/
│       └── main.go              # Entry point, CLI flags, signal handling
├── internal/
│   ├── config/
│   │   └── config.go            # Env-var-first config loading, optional YAML, validation
│   ├── smtp/
│   │   ├── server.go            # SMTP server (listener, session management)
│   │   ├── session.go           # Per-connection SMTP session (state machine)
│   │   └── auth.go              # SMTP AUTH PLAIN/LOGIN handler
│   ├── email/
│   │   └── message.go           # Parsed email model (headers, body, attachments)
│   ├── parser/
│   │   └── parser.go            # MIME message parser (net/mail + mime/multipart)
│   ├── tls/
│   │   └── tls.go               # Self-signed TLS certificate auto-generation
│   └── provider/
│       ├── provider.go          # Provider interface definition + auto-selection
│       ├── stdout/
│       │   └── stdout.go        # Stdout provider (default, prints emails to console)
│       └── graph/
│           ├── graph.go         # Microsoft Graph Send Mail implementation
│           ├── auth.go          # OAuth2 client credentials token management
│           └── types.go         # Graph API request/response types
├── config.example.yaml          # Example configuration file (optional usage)
├── Dockerfile                   # Multi-stage build, zero-config ready
├── go.mod
├── go.sum
└── README.md
```

## Technology Stack

- **Language**: Go 1.23+
- **SMTP**: Custom implementation using `net` package (no heavy SMTP library needed for receive-only)
- **Email Parsing**: `net/mail` + `mime` + `mime/multipart` from standard library
- **HTTP Client**: `net/http` for Graph API calls
- **OAuth2**: `golang.org/x/oauth2` with `clientcredentials` sub-package
- **TLS**: `crypto/tls`, `crypto/x509`, `crypto/ecdsa`, `crypto/elliptic`, `crypto/rand` for self-signed certificate generation
- **Configuration**: `gopkg.in/yaml.v3` for optional YAML parsing; `os.Getenv` for primary env var config
- **Logging**: `log/slog` (standard library structured logging, Go 1.21+)
- **Testing**: `testing` package + `net/smtp` client for integration tests

### Dependency Summary

| Dependency | Purpose | Justification |
|---|---|---|
| `golang.org/x/oauth2` | OAuth2 client credentials flow | Standard Go OAuth2 library, well-maintained |
| `gopkg.in/yaml.v3` | Optional YAML config parsing | De facto standard for YAML in Go |

No additional frameworks. The standard library covers SMTP protocol handling, HTTP client, MIME parsing, JSON encoding, structured logging, and TLS certificate generation.

## Milestones

### Phase 1: SMTP Server + Message Parsing + TLS (Primary Goal)

**Scope**: Core SMTP receive capability, email parsing, and auto-TLS.

- Implement TCP listener with configurable address (default `:2525`)
- Build SMTP session state machine (greeting, EHLO, MAIL FROM, RCPT TO, DATA, QUIT)
- Implement SMTP AUTH PLAIN and LOGIN (enabled only when `SMTP_USERNAME`/`SMTP_PASSWORD` are set)
- Implement TLS auto-generation: generate self-signed ECDSA certificate in memory on startup
- Support custom TLS certificates via `TLS_CERT_FILE` and `TLS_KEY_FILE` env vars
- Advertise STARTTLS in EHLO response
- Parse received RFC 5322 messages using `net/mail`
- Parse MIME multipart bodies and extract attachments
- Define `Email` model struct (From, To, Cc, Bcc, Subject, TextBody, HtmlBody, Attachments)
- Unit tests for SMTP session state transitions
- Unit tests for MIME parsing with various content types
- Unit tests for TLS certificate generation

**Dependencies**: None (foundation layer)

### Phase 2: Provider Interface + Stdout Provider (Secondary Goal)

**Scope**: Provider abstraction and default stdout provider.

- Define `Provider` interface: `Send(ctx context.Context, msg *Email) error`
- Implement Stdout provider (default, no-config provider):
  - Print From, To, Cc, Subject, Body preview to stdout
  - Print attachment names and sizes
  - Always return success
- Implement provider auto-selection logic:
  - If all Graph API env vars are set -> Graph provider
  - Otherwise -> Stdout provider
- Unit tests for Stdout provider output format
- Unit tests for provider selection logic

**Dependencies**: Phase 1 (Email model)

### Phase 3: Graph Provider + Configuration (Secondary Goal)

**Scope**: Microsoft Graph API integration and env-var-first configuration.

- Implement env-var-first configuration loader:
  - Read all env vars with defaults (see R5.2 table)
  - Optionally load YAML config file as base (env vars override)
  - Auto-detect provider based on presence of Graph credentials
- Implement OAuth2 client credentials token acquisition and caching
- Implement Graph Send Mail API call (`POST /users/{sender}/sendMail`)
- Handle attachment base64 encoding for Graph API format
- Implement retry logic with exponential backoff for transient errors (429, 5xx)
- Handle token refresh on 401 responses
- Wire all components together in `main.go`
- Implement graceful shutdown (SIGTERM/SIGINT handling)
- Unit tests for Graph request construction
- Unit tests for retry logic and error classification
- Integration test: end-to-end SMTP receive to provider call

**Dependencies**: Phase 1, Phase 2

### Phase 4: Docker + Tests + Polish (Final Goal)

**Scope**: Production readiness and zero-config Docker deployment.

- Create multi-stage Dockerfile:
  - Build stage: Go build with `CGO_ENABLED=0`
  - Runtime stage: scratch or distroless base
  - No ENTRYPOINT env vars required (zero-config startup)
  - Document `docker run` with zero config (stdout mode)
  - Document `docker run` with Graph API env vars (production mode)
- Integration tests with mock Graph API server
- End-to-end test: send email via `net/smtp` client, verify provider output
- Zero-config startup test: verify service starts with no env vars
- Create `config.example.yaml` with documented fields (optional usage)
- Create README with:
  - Quick start: `docker run` with no config
  - Production setup: `docker run` with Graph API env vars
  - All environment variables documented
  - Optional YAML config file usage
- Optional: `/healthz` HTTP endpoint
- Optional: Prometheus metrics endpoint

**Dependencies**: Phase 1-3

## Architecture Design

```
┌─────────────┐     ┌──────────────┐     ┌──────────┐     ┌─────────────────────┐
│ SMTP Client │────>│ SMTP Server  │────>│  Parser  │────>│   Provider          │
│ (legacy app)│     │ (net.Listener│     │ (net/mail │     │   Interface         │
│             │<────│  + sessions  │     │  + mime)  │     │                     │
│             │     │  + auto-TLS) │     └──────────┘     │ ┌─────────────────┐ │
│   250 OK    │     └──────────────┘                      │ │ Stdout Provider │ │
│   or error  │                                           │ │ (default, no    │ │
└─────────────┘                                           │ │  config needed) │ │
                                                          │ └─────────────────┘ │
                    ┌──────────────┐                       │ ┌─────────────────┐ │
                    │   Config     │                       │ │ Graph Provider  │ │
                    │ (env vars    │──────────────────────>│ │ (when Graph API │ │
                    │  primary,    │  selects provider     │ │  creds are set) │ │
                    │  YAML opt.)  │                       │ └─────────────────┘ │
                    └──────────────┘                       │ ┌─────────────────┐ │
                                                          │ │ (Future:        │ │
                                                          │ │  SES, etc.)     │ │
                                                          │ └─────────────────┘ │
                                                          └─────────────────────┘
```

**Data Flow**:
1. SMTP client connects to the server
2. Server offers STARTTLS (auto-generated or custom certificate)
3. SMTP session handles protocol commands (EHLO, AUTH if enabled, MAIL FROM, RCPT TO, DATA)
4. On DATA completion, raw message bytes are passed to the parser
5. Parser produces an `Email` struct with headers, body, and attachments
6. `Email` is passed to the auto-selected `Provider.Send()` method:
   - **Stdout provider** (default): prints email content to stdout
   - **Graph provider** (when credentials set): sends via Microsoft Graph API
7. Result (success/error) is mapped back to SMTP response code

**Key Design Decisions**:
- **Zero-config startup**: The service works with `docker run` and zero env vars. Stdout provider + auto-TLS + no auth = fully functional SMTP receiver for testing.
- **Env-var-first configuration**: Environment variables are the primary configuration method. YAML config file is an optional override for users who prefer file-based configuration.
- **Auto-TLS**: Self-signed certificate is generated in memory on startup, enabling STARTTLS without any manual certificate management.
- **Stdout as default provider**: When no Graph credentials are set, emails are printed to stdout. This makes the service useful for testing and debugging with zero setup.
- **No persistent queue**: Relay-only design keeps the system simple. If Graph API is down, the SMTP client receives an error and can retry.
- **Provider interface**: Clean separation allows adding SES, SendGrid, or other backends without touching SMTP or parsing code.
- **Standard library first**: Minimizes dependencies and binary size. Only `golang.org/x/oauth2` and `gopkg.in/yaml.v3` are external.
- **slog for logging**: Built-in structured logging avoids dependency on zap/zerolog while providing JSON output.

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Graph API rate limiting | Emails fail with 429 | Exponential backoff retry (R3.5) |
| OAuth2 token expiration mid-request | Single email fails | Token refresh on 401 + pre-expiry refresh (R3.7, R3.8) |
| Large attachments exceed Graph limits | Email rejected | Validate size before sending, return clear SMTP error |
| Malformed MIME messages | Parse failure | Graceful degradation: log warning, skip unparseable parts (R2.4) |
| SMTP clients sending very large messages | Memory pressure | Configurable max message size via `SMTP_MAX_MESSAGE_SIZE` env var (default 25 MB) |
| Self-signed TLS cert rejected by clients | STARTTLS negotiation fails | Clients can skip verification; users can provide own cert via env vars |
| Misconfigured Graph credentials | Silent email loss | Log provider selection on startup; stdout fallback makes misconfiguration visible |
