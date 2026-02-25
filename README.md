# SMTP Proxy Lite

Lightweight SMTP-to-API proxy that accepts standard SMTP traffic and delivers emails through the Microsoft Graph API or AWS SES.

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

### Microsoft Graph API

```bash
docker run -p 2525:2525 \
  -e PROVIDER=graph \
  -e GRAPH_TENANT_ID=your-tenant-id \
  -e GRAPH_CLIENT_ID=your-client-id \
  -e GRAPH_CLIENT_SECRET=your-client-secret \
  -e GRAPH_SENDER=noreply@yourdomain.com \
  -e SMTP_USERNAME=myuser \
  -e SMTP_PASSWORD=mypassword \
  smtp-proxy-lite
```

#### Azure AD App Registration

1. Register an application in Azure AD
2. Add the `Mail.Send` application permission under Microsoft Graph
3. Grant admin consent for the permission
4. Create a client secret
5. Set `GRAPH_SENDER` to the mailbox the app will send from

### AWS SES

```bash
docker run -p 2525:2525 \
  -e PROVIDER=ses \
  -e SES_REGION=us-east-1 \
  -e SES_SENDER=noreply@yourdomain.com \
  -e SES_ACCESS_KEY_ID=your-access-key \
  -e SES_SECRET_ACCESS_KEY=your-secret-key \
  -e SMTP_USERNAME=myuser \
  -e SMTP_PASSWORD=mypassword \
  smtp-proxy-lite
```

#### AWS SES Setup

1. Verify your sender email address or domain in the SES console
2. Create IAM credentials with `ses:SendEmail` and `ses:SendRawEmail` permissions
3. If running on AWS (EC2/ECS/Lambda), you can omit `SES_ACCESS_KEY_ID` and `SES_SECRET_ACCESS_KEY` to use the default credential chain (IAM roles)

## Environment Variables

| Variable | Description | Default |
|---|---|---|
| `PROVIDER` | Email provider: `stdout`, `graph`, `ses` | `` (auto-detect) |
| `SMTP_LISTEN` | Address to listen on | `:2525` |
| `SMTP_USERNAME` | SMTP AUTH username (empty = auth disabled) | `` |
| `SMTP_PASSWORD` | SMTP AUTH password (empty = auth disabled) | `` |
| `SMTP_MAX_MESSAGE_SIZE` | Maximum message size in bytes | `26214400` (25 MB) |
| `GRAPH_TENANT_ID` | Azure AD tenant ID | `` |
| `GRAPH_CLIENT_ID` | Azure AD application (client) ID | `` |
| `GRAPH_CLIENT_SECRET` | Azure AD client secret | `` |
| `GRAPH_SENDER` | Email address to send from (Graph) | `` |
| `SES_REGION` | AWS region for SES | `` |
| `SES_ACCESS_KEY_ID` | AWS access key ID (optional, uses default credential chain) | `` |
| `SES_SECRET_ACCESS_KEY` | AWS secret access key (optional) | `` |
| `SES_SENDER` | Email address to send from (SES) | `` |
| `TLS_CERT_FILE` | Path to TLS certificate file | `` (auto-generate) |
| `TLS_KEY_FILE` | Path to TLS private key file | `` (auto-generate) |
| `LOG_LEVEL` | Log level: debug, info, warn, error | `info` |

### Provider Selection

When `PROVIDER` is set explicitly, that provider is used (and required env vars are validated). When `PROVIDER` is not set, auto-detection is used: Graph if all Graph env vars are set, then SES if region and sender are set, otherwise stdout.

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
     | MIME Parser     |  | Envelope Data |
     | (RFC 5322)      |  | (MAIL/RCPT)  |
     +--------+--------+  +-------+-------+
              |                    |
              +--------+-----------+
                       |
         +-------------+-------------+
         |             |             |
         v             v             v
  +-----------+  +-----------+  +-----------+
  |  Stdout   |  |  Graph    |  |  AWS SES  |
  |  Provider |  |  Provider |  |  Provider |
  +-----------+  +-----+-----+  +-----+-----+
                       |              |
                       v              v
               +------------+  +------------+
               | MS Graph   |  | AWS SES    |
               | API        |  | API (v2)   |
               +------------+  +------------+
```

## License

MIT
