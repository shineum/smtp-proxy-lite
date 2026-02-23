---
id: SPEC-SMTP-001
version: 0.2.0
status: draft
created: 2026-02-23
updated: 2026-02-23
author: MoAI
priority: high
tags: [smtp, proxy, microsoft-graph, email, docker, zero-config]
lifecycle: spec-first
---

# SPEC-SMTP-001: SMTP Proxy Lite - SMTP to Microsoft Graph Bridge

## Overview

SMTP Proxy Lite is a lightweight Go service that receives emails via standard SMTP protocol and forwards them through the Microsoft Graph API (Send Mail endpoint). It bridges legacy SMTP-speaking systems (monitoring tools, legacy applications, scheduled jobs) with modern cloud-based email delivery, eliminating the need for a full MTA deployment.

The architecture uses an abstract provider interface so additional email backends (AWS SES, SendGrid, etc.) can be added in the future without modifying core SMTP handling logic.

**Design Philosophy**: Zero-configuration startup. The service works out of the box with `docker run` -- with no environment variables set, it starts in stdout mode (printing emails to the console) with an auto-generated self-signed TLS certificate and no authentication. Adding Graph API credentials via environment variables upgrades it to a production email relay.

## Environment

- **Runtime**: Go 1.23+ single binary
- **Deployment**: Docker container or standalone binary
- **Dependencies**: Microsoft Entra ID (Azure AD) application registration with Mail.Send permission (only when using Graph provider)
- **Network**: Listens on a configurable TCP port for inbound SMTP connections
- **Configuration**: Environment variables (primary), YAML configuration file (optional override)

## Assumptions

- A1: The service operates as a relay-only SMTP server; it does not store or queue emails persistently.
- A2: Microsoft Graph API credentials (tenant ID, client ID, client secret) are optional. When not provided, the service falls back to stdout provider for testing/development.
- A3: The service handles one email transaction per SMTP connection (no pipelining required for v1).
- A4: Attachment size is bounded by Microsoft Graph API limits (up to 150 MB per message, 4 MB inline / 35 MB with upload session for attachments) when using the Graph provider.
- A5: The service auto-generates a self-signed TLS certificate on startup if no certificate files are provided, enabling STARTTLS support out of the box.
- A6: OAuth2 client credentials flow is used (application-level permissions, no user delegation) when using the Graph provider.
- A7: Environment variables are the primary configuration method. A YAML configuration file is supported as an optional override mechanism.
- A8: The service is designed for Docker-first deployment with sensible defaults for all settings, requiring zero configuration to start.

## Requirements

### R1: SMTP Server

**R1.1** (Ubiquitous) The SMTP server **shall** listen on a configurable TCP address and port (default `:2525`).

**R1.2** (Event-Driven) **When** a client connects, the SMTP server **shall** respond with a `220` greeting banner.

**R1.3** (Event-Driven) **When** the server receives an `EHLO` or `HELO` command, it **shall** respond with supported extensions including `AUTH PLAIN LOGIN` (when AUTH is enabled), `SIZE`, and `STARTTLS` (when TLS is available).

**R1.4** (Event-Driven) **When** the server receives a `MAIL FROM` command, it **shall** validate the sender address format and store it for the current transaction.

**R1.5** (Event-Driven) **When** the server receives a `RCPT TO` command, it **shall** validate the recipient address format and add it to the recipient list for the current transaction.

**R1.6** (Event-Driven) **When** the server receives a `DATA` command followed by the message body terminated by `<CRLF>.<CRLF>`, it **shall** accept the complete message and forward it to the configured email provider.

**R1.7** (State-Driven) **While** SMTP AUTH is enabled (i.e., `SMTP_USERNAME` and `SMTP_PASSWORD` environment variables are set), **when** a client issues `AUTH PLAIN` or `AUTH LOGIN`, the server **shall** verify credentials against the configured username/password.

**R1.8** (Unwanted) The SMTP server **shall not** accept relay connections from clients that fail authentication when AUTH is enabled.

**R1.9** (State-Driven) **While** SMTP AUTH is not configured (no `SMTP_USERNAME`/`SMTP_PASSWORD` set), the server **shall** operate in open relay mode, accepting email from any client without authentication.

**R1.10** (Event-Driven) **When** the provider successfully sends the email, the server **shall** respond with `250 OK` to the client.

**R1.11** (Event-Driven) **When** the provider fails to send the email, the server **shall** respond with an appropriate SMTP error code (`451` for transient errors, `550` for permanent errors).

### R2: Message Parsing

**R2.1** (Event-Driven) **When** a `DATA` payload is received, the system **shall** parse the email message into structured components: headers (From, To, Cc, Bcc, Subject, Content-Type), body (text/plain, text/html), and attachments.

**R2.2** (Event-Driven) **When** MIME multipart content is detected, the parser **shall** extract all parts including nested multipart structures.

**R2.3** (Event-Driven) **When** attachments are present, the parser **shall** extract filename, content type, and raw content for each attachment.

**R2.4** (Unwanted) The parser **shall not** silently discard unrecognized MIME parts; they **shall** be logged as warnings and skipped.

### R3: Microsoft Graph Provider

**R3.1** (State-Driven) **While** Graph API credentials are configured (`GRAPH_TENANT_ID`, `GRAPH_CLIENT_ID`, `GRAPH_CLIENT_SECRET`, and `GRAPH_SENDER` are all set), the system **shall** use the Microsoft Graph provider to deliver emails.

**R3.2** (Ubiquitous) The Microsoft Graph provider **shall** authenticate using OAuth2 client credentials flow with tenant ID, client ID, and client secret.

**R3.3** (Event-Driven) **When** a parsed email is passed to the Graph provider, it **shall** construct a Send Mail API request (`POST /users/{sender}/sendMail`) with the email content.

**R3.4** (Event-Driven) **When** attachments are present, the provider **shall** base64-encode each attachment and include it in the `attachments` array of the Graph API request body.

**R3.5** (Event-Driven) **When** the Graph API returns a transient error (HTTP 429, 503, or 5xx), the provider **shall** retry the request up to 3 times with exponential backoff (1s, 2s, 4s).

**R3.6** (Event-Driven) **When** the Graph API returns a permanent error (HTTP 400, 401, 403), the provider **shall** return the error immediately without retry.

**R3.7** (Event-Driven) **When** receiving HTTP 401 Unauthorized, the provider **shall** attempt to refresh the OAuth2 token once before returning a permanent error.

**R3.8** (Ubiquitous) The provider **shall** cache the OAuth2 access token and refresh it before expiration.

### R4: Provider Interface

**R4.1** (Ubiquitous) The system **shall** define a `Provider` interface with a `Send(ctx context.Context, msg *Email) error` method.

**R4.2** (Ubiquitous) The system **shall** include two built-in providers: Stdout (default) and Microsoft Graph.

**R4.3** (Ubiquitous) The system **shall** automatically select the provider based on configuration: if Graph API credentials are fully configured, use Graph provider; otherwise, use Stdout provider.

**R4.4** (Optional) **Where** additional providers are needed, the system **shall** support registering new providers by implementing the `Provider` interface.

### R5: Configuration

**R5.1** (Ubiquitous) The system **shall** use environment variables as the primary configuration method. All settings **shall** have sensible defaults so the service works with zero configuration.

**R5.2** (Ubiquitous) The system **shall** support the following environment variables:

| Variable | Default | Description |
|---|---|---|
| `SMTP_LISTEN` | `:2525` | SMTP server listen address |
| `SMTP_USERNAME` | _(empty, AUTH disabled)_ | SMTP AUTH username |
| `SMTP_PASSWORD` | _(empty, AUTH disabled)_ | SMTP AUTH password |
| `SMTP_MAX_MESSAGE_SIZE` | `26214400` (25 MB) | Maximum message size in bytes |
| `GRAPH_TENANT_ID` | _(empty)_ | Microsoft Entra tenant ID |
| `GRAPH_CLIENT_ID` | _(empty)_ | Microsoft Entra application (client) ID |
| `GRAPH_CLIENT_SECRET` | _(empty)_ | Microsoft Entra client secret |
| `GRAPH_SENDER` | _(empty)_ | Sender email address for Graph API |
| `TLS_CERT_FILE` | _(empty, auto-generate)_ | Path to custom TLS certificate file |
| `TLS_KEY_FILE` | _(empty, auto-generate)_ | Path to custom TLS private key file |
| `LOG_LEVEL` | `info` | Log level (debug, info, warn, error) |

**R5.3** (Optional) **Where** a YAML configuration file is provided via `--config` CLI flag or `SMTP_PROXY_CONFIG` environment variable, the system **shall** load it as a base configuration.

**R5.4** (Ubiquitous) Environment variables **shall** always override YAML configuration values when both are present.

**R5.5** (State-Driven) **If** no Graph API credentials are configured (any of `GRAPH_TENANT_ID`, `GRAPH_CLIENT_ID`, `GRAPH_CLIENT_SECRET`, or `GRAPH_SENDER` is missing), **then** the system **shall** fall back to the Stdout provider and log a message indicating stdout mode is active.

**R5.6** (State-Driven) **If** a sender address is configured via `GRAPH_SENDER`, **then** the system **shall** use that address as the Graph API sender regardless of the SMTP `MAIL FROM` address.

**R5.7** (Ubiquitous) The system **shall** start successfully with zero configuration, using all default values (stdout provider, auto-generated TLS, no authentication, port 2525).

### R6: Logging and Error Handling

**R6.1** (Ubiquitous) The system **shall** use structured logging (JSON format) with configurable log levels (debug, info, warn, error).

**R6.2** (Event-Driven) **When** the service starts, it **shall** log the active provider (stdout or msgraph), listen address, and whether AUTH and TLS are enabled.

**R6.3** (Event-Driven) **When** an email is successfully forwarded, the system **shall** log the transaction with message ID, sender, recipients, and timestamp at info level.

**R6.4** (Event-Driven) **When** an error occurs during Graph API communication, the system **shall** log the error with request details, response status, and correlation ID at error level.

**R6.5** (Unwanted) The system **shall not** log sensitive data (OAuth tokens, client secrets, full email bodies) at any log level.

### R7: TLS Auto-Generation

**R7.1** (State-Driven) **While** no custom TLS certificate is provided (neither `TLS_CERT_FILE` nor `TLS_KEY_FILE` is set), **when** the SMTP server starts, it **shall** automatically generate a self-signed TLS certificate and private key in memory.

**R7.2** (Ubiquitous) The auto-generated TLS certificate **shall** enable STARTTLS support for SMTP connections out of the box.

**R7.3** (State-Driven) **While** custom TLS certificate files are provided via `TLS_CERT_FILE` and `TLS_KEY_FILE` environment variables, the server **shall** use those files instead of generating a self-signed certificate.

**R7.4** (Event-Driven) **When** custom TLS certificate files are specified but cannot be loaded (file not found, invalid format), the system **shall** exit with a clear error message.

**R7.5** (Unwanted) The system **shall not** write the auto-generated certificate or key to disk; they **shall** exist only in memory for the lifetime of the process.

### R8: Stdout Provider

**R8.1** (Ubiquitous) The Stdout provider **shall** be the default provider when no Graph API credentials are configured.

**R8.2** (Event-Driven) **When** a parsed email is passed to the Stdout provider, it **shall** print the email content to stdout in a human-readable format including: From, To, Cc (if present), Subject, Body (text, truncated if very long), and a list of attachment names and sizes (if present).

**R8.3** (Ubiquitous) The Stdout provider **shall** always return success (no errors) to enable easy testing and development.

**R8.4** (Event-Driven) **When** the Stdout provider receives an email, it **shall** log the email transaction at info level, consistent with the Graph provider logging behavior.

**R8.5** (Unwanted) The Stdout provider **shall not** attempt any network communication; it is a local-only output mechanism.

## Non-Functional Requirements

**NF1** (Ubiquitous) The service **shall** be deployable as a single static binary with no external runtime dependencies.

**NF2** (Ubiquitous) The service **shall** provide a `Dockerfile` for containerized deployment.

**NF3** (Ubiquitous) The service **shall** work out of the box with `docker run <image>` requiring zero environment variables or configuration files (zero-config startup).

**NF4** (Event-Driven) **When** receiving `SIGTERM` or `SIGINT`, the service **shall** perform graceful shutdown: stop accepting new connections, complete in-flight transactions (with 30-second timeout), then exit.

**NF5** (Optional) **Where** health checking is needed, the service **shall** expose a `/healthz` HTTP endpoint on a configurable port.

## Traceability

| Requirement | Plan Reference | Acceptance Reference |
|-------------|----------------|---------------------|
| R1 (SMTP Server) | Phase 1 | AC-1, AC-3 |
| R2 (Message Parsing) | Phase 1 | AC-1, AC-2 |
| R3 (Graph Provider) | Phase 3 | AC-1, AC-2, AC-4 |
| R4 (Provider Interface) | Phase 2 | AC-1, AC-6 |
| R5 (Configuration) | Phase 3 | AC-5 |
| R6 (Logging) | Phase 1-4 | AC-1, AC-4 |
| R7 (TLS Auto-Generation) | Phase 1 | AC-8 |
| R8 (Stdout Provider) | Phase 2 | AC-6, AC-7 |
| NF1-NF5 | Phase 4 | AC-5, AC-6 |
