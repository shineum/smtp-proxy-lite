---
id: SPEC-SMTP-001
type: acceptance
spec_ref: SPEC-SMTP-001
---

# Acceptance Criteria: SMTP Proxy Lite

## AC-1: Simple Text Email Forwarding

**Scenario**: A legacy system sends a plain text email through the SMTP proxy, and it is delivered via Microsoft Graph API.

```gherkin
Given the SMTP proxy is running with valid Graph API credentials:
    | Variable            | Value                |
    | GRAPH_TENANT_ID     | tenant-uuid          |
    | GRAPH_CLIENT_ID     | client-uuid          |
    | GRAPH_CLIENT_SECRET | client-secret-value  |
    | GRAPH_SENDER        | noreply@example.com  |
  And SMTP AUTH is enabled with:
    | Variable       | Value  |
    | SMTP_USERNAME  | relay  |
    | SMTP_PASSWORD  | secret |
When a client connects to the SMTP proxy on port 2525
  And authenticates with AUTH PLAIN using "relay" / "secret"
  And sends MAIL FROM:<sender@example.com>
  And sends RCPT TO:<recipient@example.com>
  And sends DATA with a plain text email:
    | Header  | Value                        |
    | From    | sender@example.com           |
    | To      | recipient@example.com        |
    | Subject | Test Email                   |
    | Body    | Hello, this is a test email. |
Then the proxy shall parse the email headers and body
  And the proxy shall call Graph API POST /users/{sender}/sendMail
  And the Graph API request body shall contain:
    | Field                        | Value                        |
    | message.subject              | Test Email                   |
    | message.body.contentType     | text                         |
    | message.body.content         | Hello, this is a test email. |
    | message.toRecipients[0].email| recipient@example.com        |
  And the proxy shall respond to the SMTP client with "250 OK"
  And the transaction shall be logged at info level with message ID, sender, and recipient
```

## AC-2: Email with Attachments

**Scenario**: An email with file attachments is forwarded with attachments properly encoded for Graph API.

```gherkin
Given the SMTP proxy is running with valid Graph API credentials
When a client sends an email with a MIME multipart message containing:
    | Part          | Content-Type        | Filename    | Size   |
    | text body     | text/plain          | -           | -      |
    | html body     | text/html           | -           | -      |
    | attachment 1  | application/pdf     | report.pdf  | 500 KB |
    | attachment 2  | image/png           | logo.png    | 50 KB  |
Then the proxy shall parse the multipart MIME structure
  And the proxy shall extract both the text/plain and text/html body parts
  And the proxy shall extract 2 attachments with correct filenames and content types
  And the Graph API request shall include attachments as base64-encoded items:
    | name        | contentType         | contentBytes  |
    | report.pdf  | application/pdf     | <base64 data> |
    | logo.png    | image/png           | <base64 data> |
  And the proxy shall respond with "250 OK"
```

## AC-3: Invalid SMTP AUTH Credentials

**Scenario**: A client attempts to relay email with wrong credentials and is rejected.

```gherkin
Given the SMTP proxy is running with SMTP AUTH enabled:
    | Variable       | Value  |
    | SMTP_USERNAME  | relay  |
    | SMTP_PASSWORD  | secret |
When a client connects and sends EHLO
Then the server shall advertise AUTH PLAIN LOGIN

When the client sends AUTH PLAIN with username "relay" and password "wrong"
Then the server shall respond with "535 Authentication failed"
  And the server shall not accept any MAIL FROM commands
  And the failed authentication attempt shall be logged at warn level

When the client sends AUTH LOGIN with username "unknown" and password "secret"
Then the server shall respond with "535 Authentication failed"
```

## AC-4: Graph API Failure Handling

**Scenario**: The Graph API returns errors, and the proxy handles them with appropriate retry and SMTP error responses.

### AC-4.1: Transient Error with Retry

```gherkin
Given the SMTP proxy is running with Graph provider active
  And the Graph API is returning HTTP 503 Service Unavailable
When a client sends a valid email through the proxy
Then the proxy shall retry the Graph API call up to 3 times
  And retry intervals shall follow exponential backoff (1s, 2s, 4s)
  And each retry shall be logged at warn level

When all 3 retries fail
Then the proxy shall respond to the SMTP client with "451 Temporary failure, please retry"
  And the final failure shall be logged at error level with correlation details
```

### AC-4.2: Rate Limiting (HTTP 429)

```gherkin
Given the SMTP proxy is running with Graph provider active
  And the Graph API returns HTTP 429 Too Many Requests with Retry-After header
When a client sends a valid email through the proxy
Then the proxy shall retry respecting the Retry-After duration
  And shall retry up to 3 times total
  And shall log each rate-limited attempt at warn level
```

### AC-4.3: Permanent Error

```gherkin
Given the SMTP proxy is running with Graph provider active
  And the Graph API returns HTTP 400 Bad Request
When a client sends a valid email through the proxy
Then the proxy shall not retry the request
  And the proxy shall respond with "550 Permanent failure"
  And the error details shall be logged at error level
```

### AC-4.4: Token Refresh on 401

```gherkin
Given the SMTP proxy is running with Graph provider active
  And the cached OAuth2 token has expired
When a client sends a valid email and Graph API returns HTTP 401 Unauthorized
Then the proxy shall refresh the OAuth2 token
  And retry the Graph API call with the new token
  And if the retry succeeds, respond with "250 OK"
  And if the retry fails with 401 again, respond with "550 Authentication error with mail provider"
```

## AC-5: Configuration Loading and Validation

### AC-5.1: Environment Variable Configuration (Primary Method)

```gherkin
Given the following environment variables are set:
    | Variable            | Value                |
    | SMTP_LISTEN         | :2525                |
    | SMTP_USERNAME       | relay                |
    | SMTP_PASSWORD       | secret               |
    | GRAPH_TENANT_ID     | tenant-uuid          |
    | GRAPH_CLIENT_ID     | client-uuid          |
    | GRAPH_CLIENT_SECRET | client-secret-value  |
    | GRAPH_SENDER        | noreply@example.com  |
    | LOG_LEVEL           | info                 |
When the proxy starts with no --config flag
Then the proxy shall load all configuration from environment variables
  And select the Graph provider (all Graph credentials present)
  And enable SMTP AUTH (username and password are set)
  And start listening on port 2525
  And log startup info including: provider=msgraph, listen=:2525, auth=enabled, tls=auto
```

### AC-5.2: Optional YAML Configuration with Env Override

```gherkin
Given a config.yaml file exists with:
    """yaml
    smtp:
      listen: ":3025"
      auth:
        username: "yaml-user"
        password: "yaml-pass"

    provider:
      msgraph:
        tenant_id: "yaml-tenant"
        client_id: "yaml-client"
        client_secret: "yaml-secret"
        sender: "yaml@example.com"

    logging:
      level: "debug"
    """
  And environment variable GRAPH_CLIENT_SECRET is set to "env-secret"
  And environment variable LOG_LEVEL is set to "warn"
When the proxy starts with --config config.yaml
Then the proxy shall use "env-secret" as the client secret (env overrides YAML)
  And the proxy shall use "warn" as the log level (env overrides YAML)
  And the proxy shall use "yaml-tenant" as the tenant ID (YAML value, no env override)
  And the proxy shall use ":3025" as the listen address (YAML value, no env override)
```

### AC-5.3: Missing Optional Config File

```gherkin
Given no config file exists at the specified path
When the proxy starts with --config nonexistent.yaml
Then the proxy shall exit with code 1
  And print an error message: "failed to load config: open nonexistent.yaml: no such file or directory"
```

### AC-5.4: Sender Address Mapping

```gherkin
Given GRAPH_SENDER is set to "noreply@example.com"
When a client sends MAIL FROM:<app@internal.local>
Then the proxy shall use "noreply@example.com" as the Graph API sender address
  And the email "From" header shall remain as "app@internal.local"
```

## AC-6: Zero-Config Startup

**Scenario**: The service starts with absolutely no configuration and is immediately functional in stdout mode.

```gherkin
Given no environment variables are set (no GRAPH_*, no SMTP_*, no TLS_*, no LOG_*)
  And no config file is provided
When the proxy starts (e.g., docker run smtp-proxy-lite)
Then the proxy shall start successfully with exit code 0
  And listen on the default address :2525
  And use the Stdout provider (no Graph credentials detected)
  And generate a self-signed TLS certificate in memory
  And advertise STARTTLS in EHLO response
  And operate in open relay mode (no SMTP AUTH required)
  And log startup info including: provider=stdout, listen=:2525, auth=disabled, tls=auto
```

### AC-6.1: Zero-Config Email Delivery

```gherkin
Given the proxy is running in zero-config mode (stdout provider)
When a client connects on port 2525
  And sends EHLO, MAIL FROM, RCPT TO, and DATA with a test email
Then the proxy shall accept the email without requiring authentication
  And the proxy shall print the email content to stdout
  And the proxy shall respond with "250 OK"
```

## AC-7: Stdout Provider Output Format

**Scenario**: The Stdout provider prints email content in a readable format to stdout.

```gherkin
Given the proxy is running with the Stdout provider
When a client sends an email with:
    | Field       | Value                              |
    | From        | sender@example.com                 |
    | To          | alice@example.com, bob@example.com  |
    | Cc          | carol@example.com                  |
    | Subject     | Monthly Report                     |
    | Body (text) | Please find the report attached.   |
    | Attachment  | report.pdf (1.2 MB)                |
    | Attachment  | summary.xlsx (45 KB)               |
Then the stdout output shall include:
    | Line Pattern                                    |
    | From: sender@example.com                        |
    | To: alice@example.com, bob@example.com          |
    | Cc: carol@example.com                           |
    | Subject: Monthly Report                         |
    | Body: Please find the report attached.          |
    | Attachments: report.pdf (1.2 MB), summary.xlsx (45 KB) |
  And the proxy shall respond with "250 OK"
  And the transaction shall be logged at info level
```

### AC-7.1: Stdout Provider with No Attachments

```gherkin
Given the proxy is running with the Stdout provider
When a client sends a plain text email with no attachments
Then the stdout output shall include From, To, Subject, and Body
  And the stdout output shall not include an Attachments line
  And the proxy shall respond with "250 OK"
```

## AC-8: TLS Auto-Generation

**Scenario**: The service auto-generates a self-signed TLS certificate when no custom certificate is provided.

### AC-8.1: Auto-Generated Certificate

```gherkin
Given no TLS_CERT_FILE or TLS_KEY_FILE environment variables are set
When the proxy starts
Then the proxy shall generate a self-signed TLS certificate in memory
  And the EHLO response shall include STARTTLS as a supported extension
  And a client that issues STARTTLS shall successfully upgrade to TLS
  And the auto-generated certificate shall not be written to disk
```

### AC-8.2: Custom Certificate

```gherkin
Given TLS_CERT_FILE is set to "/certs/server.crt"
  And TLS_KEY_FILE is set to "/certs/server.key"
  And both files exist and contain valid certificate and key
When the proxy starts
Then the proxy shall use the custom certificate for STARTTLS
  And shall not generate a self-signed certificate
  And the EHLO response shall include STARTTLS
```

### AC-8.3: Invalid Custom Certificate

```gherkin
Given TLS_CERT_FILE is set to "/certs/server.crt"
  And TLS_KEY_FILE is set to "/certs/server.key"
  And the certificate file does not exist
When the proxy starts
Then the proxy shall exit with code 1
  And print an error message indicating the TLS certificate file could not be loaded
```

## Edge Cases

### EC-1: Oversized Message

```gherkin
Given the proxy has a configured max message size of 25 MB (default SMTP_MAX_MESSAGE_SIZE)
When a client sends DATA with a message exceeding 25 MB
Then the server shall respond with "552 Message size exceeds limit"
  And the message shall not be forwarded to the provider
```

### EC-2: Malformed Email

```gherkin
Given a client sends DATA with a malformed MIME structure
When the parser encounters an unparseable MIME part
Then the parser shall log a warning with details of the malformed part
  And skip the malformed part
  And continue processing remaining valid parts
  And forward the email with successfully parsed content
```

### EC-3: Connection Timeout

```gherkin
Given a client connects to the SMTP proxy
When the client does not send any command for 60 seconds
Then the server shall close the connection with "421 Connection timed out"
  And free all resources associated with the session
```

### EC-4: Graceful Shutdown

```gherkin
Given the SMTP proxy is running and processing an active email transaction
When the proxy receives SIGTERM
Then the proxy shall stop accepting new connections
  And wait up to 30 seconds for the active transaction to complete
  And then shut down cleanly with exit code 0
  And log "SMTP proxy shutdown complete" at info level
```

### EC-5: Multiple Recipients

```gherkin
Given a client sends an email with multiple recipients:
    | Command                          |
    | RCPT TO:<alice@example.com>      |
    | RCPT TO:<bob@example.com>        |
    | RCPT TO:<carol@example.com>      |
When the email is forwarded to the active provider
Then the provider shall receive all 3 recipients
  And the proxy shall respond with "250 OK" once
```

### EC-6: Partial Graph Credentials

```gherkin
Given GRAPH_TENANT_ID is set to "tenant-uuid"
  And GRAPH_CLIENT_ID is set to "client-uuid"
  But GRAPH_CLIENT_SECRET is not set
  And GRAPH_SENDER is not set
When the proxy starts
Then the proxy shall fall back to the Stdout provider (incomplete Graph credentials)
  And log a warning: "Incomplete Graph API credentials, using stdout provider"
  And start successfully
```

### EC-7: Open Relay Mode

```gherkin
Given no SMTP_USERNAME or SMTP_PASSWORD environment variables are set
When a client connects and sends EHLO
Then the server shall not advertise AUTH PLAIN LOGIN
  And the client shall be able to send MAIL FROM and DATA without authentication
  And the email shall be forwarded to the active provider
```

## Quality Gates

| Gate | Criteria | Method |
|------|----------|--------|
| Unit Test Coverage | >= 85% for internal/ packages | `go test -coverprofile` |
| Integration Tests | SMTP send + mock Graph API | `net/smtp` client + `httptest.Server` |
| Zero-Config Test | Service starts with no env vars | Automated startup test |
| Stdout Provider Test | Email printed to stdout correctly | Capture stdout, verify format |
| TLS Auto-Gen Test | STARTTLS works with auto cert | `net/smtp` TLS handshake test |
| No Race Conditions | Pass with `-race` flag | `go test -race ./...` |
| No Lint Issues | Zero issues from golangci-lint | `golangci-lint run` |
| Binary Size | < 20 MB static binary | `go build` with `CGO_ENABLED=0` |
| Docker Image Size | < 30 MB | Multi-stage build with scratch/distroless |
| Docker Zero-Config | `docker run` with no env starts OK | Automated Docker test |

## Definition of Done

- [ ] All acceptance criteria (AC-1 through AC-8) pass
- [ ] Edge cases (EC-1 through EC-7) are handled
- [ ] Unit test coverage >= 85%
- [ ] Integration test with mock Graph API passes
- [ ] Zero-config startup test passes (no env vars, stdout provider, auto TLS)
- [ ] Stdout provider output format matches AC-7 specification
- [ ] TLS auto-generation test passes
- [ ] `go vet` and `golangci-lint` report zero issues
- [ ] `go test -race` passes
- [ ] Dockerfile builds and runs with zero configuration
- [ ] `config.example.yaml` is documented (optional usage)
- [ ] README includes:
  - [ ] Quick start with `docker run` (zero config)
  - [ ] Production setup with env vars
  - [ ] Full environment variable reference table
  - [ ] Optional YAML config documentation
