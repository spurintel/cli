# Endpoints

## Context API Access and Setup

The Spur Context API provides a lookup capability for individual IP addresses. By default, the data returned by our API reflects what we know at this moment. It is a real-time representation of our IP address data. Accessing historical records is possible with an additional add-on subscription.

All Context API authentication is performed using a `Token` header.

```bash filename="BASE URL"
https://api.spur.us
```

### Don't have a token yet?

Visit the [Spur dashboard](https://app.spur.us) and purchase a subscription today!

### Query Arguments

| Code | Description |
| - | - |
| `dt` | A datetime in the format `YYYYmmdd`. This argument is only valid for subscriptions with access to historical records. |
| `mmgeo` | A boolean value, that when set to `1`, will return MaxMind's GeoLite2 geolocation data instead of Spur's geolocation data in the `location` object. |

### Response Codes

| Code | Description |
| - | - |
| `200` | The request succeeded. |
| `400` | Bad input provided; IP must be a public IPv4 or IPv6 address. |
| `401` | Access is denied; your token is most likely invalid. |
| `403` | Your token is valid but does not have access to the Context API. |
| `429` | Your account has insufficient queries to perform this action. |
| `500` | An error occurred. |

### Response Headers

| Code | Description |
| - | - |
| `x-balance-remaining` | The number of queries remaining in this billing cycle. |
| `x-result-dt` | The date for which this response is sourced in the format YYYYmmdd. When performing historical lookups, this date indicates that the IP context response is from the YYYYmmdd data feed. |

## The IP Context Object

The IP Context Object summarizes all available information for an IP address. It is provided as a single dictionary. All fields not explicitly marked as required may be omitted if their value is null.

> Full field reference: resource spur://schemas/ip-context

```json filename="THE IP CONTEXT OBJECT"
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

## Lookup an IP Address

Retrieves an [IP Context Object](#the-ip-context-object) by IPv4 or IPv6 address. An IP Context Object summarizes all available information about the queried IP address. Some fields are optional and will be omitted if not available.

```bash filename="GET /v2/context/:ip"
curl https://api.spur.us/v2/context/89.39.106.191 \
  -H "Token: SPUR_TOKEN"
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

## Lookup a historical IP Address

Retrieves an [IP Context Object](#the-ip-context-object) by IPv4 or IPv6 address for the specified date. An IP Context Object summarizes all available information about the queried IP address. Some fields are optional and will be omitted if not available.

> This is only available for Enterprise plans with historical access.

```bash filename="GET /v2/context/:ip?dt=YYYYmmdd"
curl https://api.spur.us/v2/context/89.39.106.191?dt=20251026 \
  -H "Token: SPUR_TOKEN"
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

## The Tag Metadata Object

The Tag Metadata Object includes analysis, statistics, and metrics for a service tag.

> Full field reference: resource spur://schemas/tag-metadata

```json filename="THE TAG METADATA OBJECT"
{
  "allowsCrypto": "false",
  "allowsFreeAccess": "false",
  "allowsMultihop": "false",
  "allowsTorrents": "false",
  "allowsWhiteLabel": "true",
  "categories": ["RESIDENTIAL_PROXY", "DATACENTER_PROXY", "MOBILE_PROXY", "ISP_PROXY"],
  "description": "OxyLabs is the second largest proxy provider tracked. Their users are sourced through SDKs embedded in mobile applications and browser extensions. These users are likely unaware that they agreed to contribute to this proxy network.",
  "isAnonymous": "true",
  "isCallbackProxy": "true",
  "isEnterprise": "false",
  "isInactive": "false",
  "isNoLog": "true",
  "metrics": {
    "averageDeviceCount": "37.20332478669546",
    "churnRate": "0.08675012801772562",
    "distinctASNs": "25334",
    "distinctCountries": "235",
    "distinctIPs": "6367903",
    "distinctISPs": "67413"
  },
  "name": "Oxylabs",
  "platforms": ["ROUTER"],
  "protocols": [],
  "tag": "OXYLABS_PROXY",
  "targetingTypes": ["CITY", "STATE", "COUNTRY", "ASN"],
  "website": "https://oxylabs.io"
}
```

## Lookup Tag Metadata

Retrieves a [Tag Metadata Object](#the-tag-metadata-object) which contains a summary of metrics and information for the provided tag.

```bash filename="GET /v2/metadata/tags/:tag"
curl https://api.spur.us/v2/metadata/tags/OXYLABS_PROXY \
  -H "Token: SPUR_TOKEN"
```

```json filename="RESPONSE"
{
  "allowsCrypto": "false",
  "allowsFreeAccess": "false",
  "allowsMultihop": "false",
  "allowsTorrents": "false",
  "allowsWhiteLabel": "true",
  "categories": ["RESIDENTIAL_PROXY", "DATACENTER_PROXY", "MOBILE_PROXY", "ISP_PROXY"],
  "description": "OxyLabs is the second largest proxy provider tracked. Their users are sourced through SDKs embedded in mobile applications and browser extensions. These users are likely unaware that they agreed to contribute to this proxy network.",
  "isAnonymous": "true",
  "isCallbackProxy": "true",
  "isEnterprise": "false",
  "isInactive": "false",
  "isNoLog": "true",
  "metrics": {
    "averageDeviceCount": "37.20332478669546",
    "churnRate": "0.08675012801772562",
    "distinctASNs": "25334",
    "distinctCountries": "235",
    "distinctIPs": "6367903",
    "distinctISPs": "67413"
  },
  "name": "Oxylabs",
  "platforms": ["ROUTER"],
  "protocols": [],
  "tag": "OXYLABS_PROXY",
  "targetingTypes": ["CITY", "STATE", "COUNTRY", "ASN"],
  "website": "https://oxylabs.io"
}
```

## Lookup API Token Status

Retrieves the status of your API token, the number of queries remaining for your billing period, and your service tier.

```bash filename="GET /status"
curl https://api.spur.us/status \
  -H "Token: SPUR_TOKEN"
```

```json filename="RESPONSE"
{
  "active": true,
  "queriesRemaining": 49283,
  "serviceTier": "online"
}
```

## Frequent Questions

### Do you have a bulk lookup endpoint?

No. We require that each IP be submitted individually.

### Where is the last-seen time?

We return the assessment as of the time of the query. We age off any data that we believe is no longer accurate.

### Do you offer historical search?

Enterprise API subscriptions have the ability to add historical lookups to their API subscription. If you are interested, please contact your Spur sales rep or [sales@spur.us](mailto:sales@spur.us).

### How do I get a token?

If you purchased our API from our website or have linked your enterprise plan to an online account, you can view your token at [https://app.spur.us/settings/api](https://app.spur.us/settings/api). Otherwise, contact your Spur rep for access information.

### How can I evaluate the API?

We do not offer a free tier, but monthly plans start at $125 from [https://app.spur.us/settings/plans](https://app.spur.us/settings/plans). If you are interested in an enterprise license, please contact [sales@spur.us](mailto:sales@spur.us).
