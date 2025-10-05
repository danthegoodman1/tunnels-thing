# tunnels-thing

Cloudflare tunnels was too complicated to figure out, so I made my own.

## Nomenclature

- **Tunnel Server**: The server that is exposed to the internet, that establishes the tunnel between the local client and the remote client
- **Remote Client**: The client that intends to connect to the local service, the thing that it can't talk to without the tunnel
- **Local Client**: The process that connects to the tunnel server to expose the local service to the internet
- **Local Service**: Your code, the thing you want to expose to the internet but can't

## Diagram

```
┌─────────────────┐
│ Remote Client   │  (wants to access local service)
│ (on internet)   │
└────────┬────────┘
         │
         │ HTTP/TCP Request
         │
         ▼
┌─────────────────┐
│ Tunnel Server   │  (exposed to internet)
│ (public IP)     │
└────────┬────────┘
         │
         │ Tunnel Connection
         │
         ▼
┌─────────────────┐
│ Local Client    │  (behind firewall/NAT)
│ (localhost)     │
└────────┬────────┘
         │
         │ Local Connection
         │
         ▼
┌─────────────────┐
│ Local Service   │  (your actual service)
│ (localhost:8080)│
└─────────────────┘
```
