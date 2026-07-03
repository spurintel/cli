# Overview

The Spur Context API provides REST API access to IP Context data. Visit our website for more information on use cases, integrations, and client successes.

## Quick Start

```bash
curl -H "Token: {{token}}" "https://api.spur.us/v2/context/IP_ADDRESS"
```

```json filename="RESPONSE"
{
  "as": {
    "number": 49981,
    "organization": "WorldStream"
  },
  "client": {
    "behaviors": ["FILE_SHARING", "TOR_PROXY_USER"],
    "concentration": {
      "city": "Polāia Kalān",
      "country": "IN",
      "density": 0.2675,
      "geohash": "tsn",
      "skew": 6762,
      "state": "Madhya Pradesh"
    },
    "count": 4,
    "countries": 2,
    "proxies": ["ABCPROXY_PROXY", "9PROXY_PROXY", "NETNUT_PROXY", "GOPROXY_PROXY"],
    "spread": 4724209,
    "types": ["MOBILE", "DESKTOP"]
  },
  "infrastructure": "DATACENTER",
  "ip": "89.39.106.191",
  "location": {
    "city": "Amsterdam",
    "country": "NL",
    "state": "North Holland"
  },
  "organization": "WorldStream B.V.",
  "risks": ["CALLBACK_PROXY", "TUNNEL", "GEO_MISMATCH"],
  "services": ["OPENVPN"],
  "tunnels": [
    {
      "anonymous": true,
      "entries": ["89.39.106.82"],
      "operator": "PROTON_VPN",
      "type": "VPN"
    }
  ]
}
```
