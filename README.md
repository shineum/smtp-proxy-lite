# SMTP Proxy Lite

Lightweight SMTP-to-API proxy that accepts standard SMTP traffic and delivers emails through the Microsoft Graph API.

## Quick Start

Zero-config mode (prints emails to stdout):

```bash
docker run -p 2525:2525 smtp-proxy-lite
```

Send a test email:

```bash
swaks --to user@example.com --from sender@example.com \
  --server localhost:2525 --body "Hello from SMTP Proxy Lite"
```

## Production Setup

Configure the Microsoft Graph API provider with environment variables:

```bash
docker run -p 2525:2525 \
  -e GRAPH_TENANT_ID=your-tenant-id \
  -e GRAPH_CLIENT_ID=your-client-id \
  -e GRAPH_CLIENT_SECRET=your-client-secret \
  -e GRAPH_SENDER=noreply@yourdomain.com \
  -e SMTP_USERNAME=myuser \
  -e SMTP_PASSWORD=mypassword \
  smtp-proxy-lite
```

### Azure AD App Registration

1. Register an application in Azure AD
2. Add the `Mail.Send` application permission under Microsoft Graph
3. Grant admin consent for the permission
4. Create a client secret
5. Set `GRAPH_SENDER` to the mailbox the app will send from

## Environment Variables

| Variable | Description | Default |
|---|---|---|
| `SMTP_LISTEN` | Address to listen on | `:2525` |
| `SMTP_USERNAME` | SMTP AUTH username (empty = auth disabled) | `` |
| `SMTP_PASSWORD` | SMTP AUTH password (empty = auth disabled) | `` |
| `SMTP_MAX_MESSAGE_SIZE` | Maximum message size in bytes | `26214400` (25 MB) |
| `GRAPH_TENANT_ID` | Azure AD tenant ID | `` |
| `GRAPH_CLIENT_ID` | Azure AD application (client) ID | `` |
| `GRAPH_CLIENT_SECRET` | Azure AD client secret | `` |
| `GRAPH_SENDER` | Email address to send from | `` |
| `TLS_CERT_FILE` | Path to TLS certificate file | `` (auto-generate) |
| `TLS_KEY_FILE` | Path to TLS private key file | `` (auto-generate) |
| `LOG_LEVEL` | Log level: debug, info, warn, error | `info` |

If `GRAPH_TENANT_ID`, `GRAPH_CLIENT_ID`, `GRAPH_CLIENT_SECRET`, and `GRAPH_SENDER` are all set, the Graph provider is used. Otherwise, emails are printed to stdout.

## Optional YAML Configuration

You can use a YAML file for base configuration. Environment variables always override YAML values.

```bash
docker run -p 2525:2525 \
  -v ./config.yaml:/config.yaml \
  smtp-proxy-lite --config /config.yaml
```

See [config.example.yaml](config.example.yaml) for all available options.

## Building from Source

```bash
go build -o smtp-proxy ./cmd/smtp-proxy
./smtp-proxy
```

Or with Docker:

```bash
docker build -t smtp-proxy-lite .
docker run -p 2525:2525 smtp-proxy-lite
```

## Architecture

```
                    SMTP Clients
                        |
                        v
                +---------------+
                |  SMTP Server  |
                |  (port 2525)  |
                +-------+-------+
                        |
                  +-----+-----+
                  |           |
                  v           v
            +---------+  +---------+
            | STARTTLS|  |  AUTH   |
            | (TLS)   |  | (PLAIN/|
            |         |  |  LOGIN)|
            +---------+  +---------+
                  |           |
                  +-----+-----+
                        |
                        v
                +---------------+
                |  MIME Parser  |
                | (RFC 5322)    |
                +-------+-------+
                        |
              +---------+---------+
              |                   |
              v                   v
     +----------------+  +----------------+
     | Stdout Provider|  | Graph Provider |
     | (development)  |  | (production)   |
     +----------------+  +-------+--------+
                                 |
                                 v
                        +----------------+
                        | Microsoft      |
                        | Graph API      |
                        | (OAuth2 + REST)|
                        +----------------+
```

## License

MIT
