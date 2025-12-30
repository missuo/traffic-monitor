# Traffic Monitor

A high-performance TCP/UDP traffic forwarding proxy with traffic statistics, written in Go.

## Features

- **TCP/UDP Forwarding**: Forward traffic between ports with minimal overhead
- **Traffic Statistics**: Track upload/download bytes (total and monthly)
- **Traffic Limits**: Set bandwidth limits per proxy, connections rejected when exceeded
- **Persistence**: Statistics survive restarts via JSON file storage
- **Multi-proxy Support**: Configure multiple forwarding rules
- **HTTP API**: Query traffic stats with Bearer token authentication
- **High Performance**: Uses buffer pooling and atomic operations

## Installation

### Binary

```bash
# Clone the repository
git clone https://github.com/missuo/traffic-monitor.git
cd traffic-monitor

# Build
go build -o traffic-monitor .
```

### Docker

```bash
docker run -d --network host \
  -v ./config.yaml:/app/config.yaml:ro \
  -v ./data:/app/data \
  ghcr.io/missuo/traffic-monitor:latest
```

### Docker Compose

```yaml
services:
  traffic-monitor:
    image: ghcr.io/missuo/traffic-monitor:latest
    container_name: traffic-monitor
    restart: unless-stopped
    network_mode: host
    volumes:
      - ./config.yaml:/app/config.yaml:ro
      - ./data:/app/data
```

## Configuration

Create a `config.yaml` file:

```yaml
api:
  port: 8080
  token: "your-secret-token"

data_file: "./data/traffic_data.json"

proxies:
  - name: "service1"
    listen_port: 10001
    target_host: "127.0.0.1"
    target_port: 10000
    protocol: "tcp"
    limit: "100GB"

  - name: "dns-proxy"
    listen_port: 5353
    target_host: "8.8.8.8"
    target_port: 53
    protocol: "udp"
    limit: "10GB"

  - name: "game-server"
    listen_port: 27015
    target_host: "192.168.1.100"
    target_port: 27015
    protocol: "both"
    limit: ""  # unlimited
```

### Configuration Options

| Field | Description | Default |
|-------|-------------|---------|
| `api.port` | HTTP API server port | `8080` |
| `api.token` | Bearer token for API authentication | `""` (no auth) |
| `data_file` | Path to persistence file | `./traffic_data.json` |
| `proxies[].name` | Unique identifier for the proxy | required |
| `proxies[].listen_port` | Port to listen on | required |
| `proxies[].target_host` | Target host to forward to | `127.0.0.1` |
| `proxies[].target_port` | Target port to forward to | required |
| `proxies[].protocol` | Protocol: `tcp`, `udp`, or `both` | `tcp` |
| `proxies[].limit` | Traffic limit (e.g., `100GB`, `1TB`) | `""` (unlimited) |

### Traffic Limit Format

Supported units: `B`, `KB`, `MB`, `GB`, `TB`

Examples:
- `"100GB"` - 100 gigabytes
- `"1.5TB"` - 1.5 terabytes
- `"500MB"` - 500 megabytes
- `""` or `"0"` - unlimited

When the limit is exceeded:
- **TCP**: New connections are rejected
- **UDP**: Packets are dropped

## Usage

```bash
# Start with default config file (config.yaml)
./traffic-monitor

# Start with custom config file
./traffic-monitor -config /path/to/config.yaml
```

## API Endpoints

### Health Check

```bash
curl http://localhost:8080/health
```

Response:
```json
{"status": "ok"}
```

### Get All Stats

```bash
curl -H "Authorization: Bearer your-secret-token" http://localhost:8080/api/stats
```

Response:
```json
{
  "proxies": [
    {
      "name": "service1",
      "protocol": "tcp",
      "listen_port": 10001,
      "target_port": 10000,
      "total": {
        "upload": 1073741824,
        "download": 2147483648,
        "upload_human": "1.00 GB",
        "download_human": "2.00 GB"
      },
      "monthly": {
        "month": "2024-12",
        "upload": 536870912,
        "download": 1073741824,
        "upload_human": "512.00 MB",
        "download_human": "1.00 GB"
      },
      "limit": 107374182400,
      "limit_human": "100.00 GB",
      "limit_exceeded": false,
      "usage": {
        "used": 3221225472,
        "used_human": "3.00 GB",
        "remaining": 104152956928,
        "remaining_human": "97.00 GB",
        "percentage": 3.0
      }
    }
  ]
}
```

### Get Stats by Proxy Name

```bash
curl -H "Authorization: Bearer your-secret-token" http://localhost:8080/api/stats/service1
```

## Performance

- **Buffer Pooling**: Reuses 32KB buffers via `sync.Pool` to reduce GC pressure
- **Atomic Operations**: Lock-free traffic counting using `sync/atomic`
- **Efficient I/O**: Uses `io.Copy` patterns for zero-copy forwarding
- **Async Persistence**: Stats saved every 30 seconds without blocking traffic

## Architecture

```
┌─────────────┐     ┌──────────────────┐     ┌─────────────┐
│   Client    │────▶│  Traffic Monitor │────▶│   Target    │
│             │◀────│   (Port 10001)   │◀────│ (Port 10000)│
└─────────────┘     └──────────────────┘     └─────────────┘
                            │
                            ▼
                    ┌──────────────┐
                    │  Stats API   │
                    │ (Port 8080)  │
                    └──────────────┘
```

## License

MIT
