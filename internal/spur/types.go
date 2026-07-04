// Package spur contains the Spur Context API client, response types, and
// embedded documentation resources.
//
// The field descriptions on exported structs mirror the upstream JSON Schemas
// at spur-web/apps/docs/src/types/ip-context.schema.json and tag-metadata.schema.json
// so that auto-generated MCP tool schemas are self-documenting.
package spur

// IPContext is the object returned by GET /v2/context/:ip. Optional fields are
// omitted when null per the upstream schema. Enum-valued string slices carry
// their allowable values (and inline descriptions) in the field description so
// they travel with every tool response.
type IPContext struct {
	AI             *AIActivity   `json:"ai,omitempty" jsonschema:"AI activity observed from this IP address. Operator is the AI entity name; types are AGENTIC (AI agent) or CRAWLER (AI crawler)."`
	AS             ASInfo        `json:"as" jsonschema:"Spur IP GeoBGP autonomous system information (ASN and operator organization)."`
	Organization   string        `json:"organization,omitempty" jsonschema:"The organization currently assigned to use the specific IP address, such as a customer, hosting provider, or service operator."`
	Client         ClientInfo    `json:"client" jsonschema:"Descriptive data about the connecting client: behaviors, geographical concentration, device types, and proxy usage."`
	Infrastructure string        `json:"infrastructure" jsonschema:"Infrastructure type. One of: DATACENTER (generic VPS), MOBILE (4G/LTE networks), SATELLITE (IP location may diverge from physical location), IN_FLIGHT_WIFI (airline transit, IP location often distinct from physical), GOOGLE (Google-hosted pool routing wifi/untrusted-mobile traffic)."`
	IP             string        `json:"ip" jsonschema:"IPv4 or IPv6 address associated with the connection."`
	Location       Location      `json:"location" jsonschema:"Spur IP Geo location information for the IP."`
	Risks          []string      `json:"risks" jsonschema:"Identified risk factors. Allowed: AD_FRAUD (ad fraud signs), CALLBACK_PROXY (can route residential/malware proxy traffic), GEO_MISMATCH (datacenter location differs from user location), LOGIN_BRUTEFORCE (persistent login attempts), TUNNEL (exit point for anonymizing VPN/proxy/tunnel), WEB_SCRAPING (automated/headless scraping)."`
	Services       []string      `json:"services" jsonschema:"Services or protocols in use. Allowed: HTTP, IPSEC, L2TP, OPENVPN, PPTP, SHADOWSOCKS, SOCKS, VMESS, V2RAY, TROJAN, WIREGUARD, PROPRIETARY (non-standard/closed-source)."`
	Tunnels        []TunnelEntry `json:"tunnels" jsonschema:"Information about tunneling services (VPN, TOR, etc.) whose exit nodes include this IP."`
}

// AIActivity describes AI operator activity observed from an IP.
type AIActivity struct {
	Operator string   `json:"operator,omitempty" jsonschema:"The name of the AI entity that is performing activity on this IP address."`
	Types    []string `json:"types,omitempty" jsonschema:"AI activity types observed. AGENTIC (performed by an AI agent) or CRAWLER (performed by an AI crawler)."`
}

// ASInfo is the AS (autonomous system) record for an IP.
type ASInfo struct {
	Number       int    `json:"number" jsonschema:"The autonomous system number."`
	Organization string `json:"organization" jsonschema:"The organization that owns and operates the Autonomous System (AS) responsible for routing the IP address via BGP."`
}

// ClientInfo summarizes connecting-client characteristics.
type ClientInfo struct {
	Behaviors     []string       `json:"behaviors" jsonschema:"Behaviors and usage patterns observed for clients. Known values include FILE_SHARING (file sharing activity), WEB_SCRAPING (scraping activity), and service-user patterns matching ^[A-Z0-9_]+_USER$ such as TOR_PROXY_USER."`
	Concentration *Concentration `json:"concentration,omitempty" jsonschema:"Geographical and density information for clients on this IP address."`
	Count         int            `json:"count" jsonschema:"Number of instances or connections from this client."`
	Countries     int            `json:"countries" jsonschema:"Number of unique countries the client connects from."`
	Proxies       []string       `json:"proxies" jsonschema:"List of proxy service tags used by the client (e.g., OXYLABS_PROXY)."`
	Spread        int            `json:"spread" jsonschema:"Integer measuring the client spread (distance/distribution)."`
	Types         []string       `json:"types" jsonschema:"Types of devices or clients connecting. Allowed: DESKTOP (standard computers/laptops), HEADLESS (no standard display), IOT (game systems, smart TVs, streaming sticks, etc.), MOBILE (phones and tablets)."`
}

// Concentration is the client's geographic concentration.
type Concentration struct {
	City    string  `json:"city" jsonschema:"Name of the city."`
	Country string  `json:"country" jsonschema:"Country code or name."`
	Density float64 `json:"density" jsonschema:"Numerical measure of the client concentration in the range 0-1."`
	Geohash string  `json:"geohash" jsonschema:"A geohash representing the location."`
	Skew    float64 `json:"skew" jsonschema:"Point-to-point distance skew from the IP location field, in kilometers."`
	State   string  `json:"state" jsonschema:"Name or code of the state."`
}

// Location is the Spur-provided geo-location of the IP.
type Location struct {
	City    string `json:"city" jsonschema:"City name or location."`
	Country string `json:"country" jsonschema:"Country code or name."`
	State   string `json:"state" jsonschema:"Name or code of the state."`
}

// TunnelEntry describes one tunneling service whose exit node includes this IP.
type TunnelEntry struct {
	Anonymous bool     `json:"anonymous" jsonschema:"Whether the tunnel operation is anonymized."`
	Entries   []string `json:"entries,omitempty" jsonschema:"Entry node IP addresses for the tunnel."`
	Exits     []string `json:"exits,omitempty" jsonschema:"Exit node IP addresses for the tunnel."`
	Operator  string   `json:"operator" jsonschema:"Operator or vendor of the tunnel service (e.g., PROTON_VPN)."`
	Type      string   `json:"type" jsonschema:"Type of tunnel, e.g., VPN, TOR."`
}

// TagMetadata is the object returned by GET /v2/metadata/tags/:tag.
//
// The upstream JSON schema (and docs) show the boolean-like fields as
// strings ("true"/"false") and the metrics fields as strings, but the live
// API returns them as bare booleans and numbers. To be robust to either
// shape we use FlexBool and FlexNumber, which tolerate both during
// unmarshal and always emit bare booleans / numbers on marshal.
//
// Optional fields use pointers so they're omitted from output when absent.
type TagMetadata struct {
	AllTags          []string    `json:"allTags,omitempty" jsonschema:"All tags associated with this entry (usually includes the tag itself plus any aliases)."`
	AllowsCrypto     *FlexBool   `json:"allowsCrypto,omitempty" jsonschema:"Whether the service supports or facilitates crypto-based payments or platforms."`
	AllowsFreeAccess *FlexBool   `json:"allowsFreeAccess,omitempty" jsonschema:"Whether the service is available for free usage."`
	AllowsMultihop   *FlexBool   `json:"allowsMultihop,omitempty" jsonschema:"Whether the service offers multi-hop or chaining functionality."`
	AllowsTorrents   *FlexBool   `json:"allowsTorrents,omitempty" jsonschema:"Whether the service permits torrent or P2P file-sharing traffic."`
	AllowsWhiteLabel *FlexBool   `json:"allowsWhiteLabel,omitempty" jsonschema:"Whether white-label or rebranded versions of the service exist."`
	Categories       []string    `json:"categories,omitempty" jsonschema:"Product categories for bandwidth reselling and routing. Allowed: BLOCKCHAIN_VPN (distributed VPN on blockchain), CDN_PROXY (routed through CDNs like Cloudflare/Fastly), COMMERCIAL_VPN (datacenter-hosted commercial VPN), DATACENTER_PROXY, ENTERPRISE_VPN (often non-anonymous), FREE_VPN, ISP_PROXY (residential/ISP networks), MOBILE_PROXY (4G/LTE), P2P_PROXY (volunteer/peer, e.g. Tor), PUBLIC_PROXY (open/unrestricted), RESIDENTIAL_PROXY (endpoints via SDK or pay-to-use programs), ZTNA_PROVIDER (zero-trust, often non-anonymizing)."`
	Description      string      `json:"description,omitempty" jsonschema:"Free-text description of the service or entity."`
	IsAnonymous      *FlexBool   `json:"isAnonymous,omitempty" jsonschema:"Whether the service primarily aims to anonymize user traffic."`
	IsCallbackProxy  *FlexBool   `json:"isCallbackProxy,omitempty" jsonschema:"Whether the service includes callback or reverse-proxy functionality."`
	IsEnterprise     *FlexBool   `json:"isEnterprise,omitempty" jsonschema:"Whether the service or platform is oriented toward enterprise usage."`
	IsInactive       *FlexBool   `json:"isInactive,omitempty" jsonschema:"Whether the service is currently inactive or defunct."`
	IsNoLog          *FlexBool   `json:"isNoLog,omitempty" jsonschema:"Whether the service claims a no-logging policy."`
	IsTracked        *FlexBool   `json:"isTracked,omitempty" jsonschema:"Whether Spur is actively tracking this service (i.e., ingesting new endpoint signal for it)."`
	MaxAgeDays       *int        `json:"maxAgeDays,omitempty" jsonschema:"Maximum age, in days, that an IP will be attributed to this service after last being observed."`
	Metrics          *TagMetrics `json:"metrics,omitempty" jsonschema:"Aggregate metrics across observed endpoints."`
	Name             string      `json:"name" jsonschema:"Human-readable name of the service or entity."`
	Platforms        []string    `json:"platforms,omitempty" jsonschema:"Operating systems and environments supported. Allowed: ANDROID, BROWSER, CUSTOM, IOS, LINUX, MACOS, ROUTER, WINDOWS."`
	Protocols        []string    `json:"protocols,omitempty" jsonschema:"Protocols or services used for network traffic. Allowed: HTTP, IPSEC, L2TP, OPENVPN, PPTP, SHADOWSOCKS, SOCKS, VMESS, V2RAY, TROJAN, WIREGUARD, PROPRIETARY."`
	ResoldBy         []string    `json:"resoldBy,omitempty" jsonschema:"Other tracked services that resell the endpoints of this service."`
	Tag              string      `json:"tag" jsonschema:"Unique identifier for this service (e.g., OXYLABS_PROXY)."`
	TargetingTypes   []string    `json:"targetingTypes,omitempty" jsonschema:"Possible granularities for configuring a service exit or route. Allowed: IP (specific IP), CIDR (any IP in a CIDR block), ASN (any IP in an ASN), CITY, STATE, COUNTRY, ZIP (typically US-only)."`
	Website          string      `json:"website,omitempty" jsonschema:"Primary website or homepage for the service."`
}

// TagMetrics are the aggregate metrics on a TagMetadata object. The upstream
// docs show these as strings, but the live API returns bare numbers;
// FlexNumber handles either.
type TagMetrics struct {
	AverageDeviceCount FlexNumber `json:"averageDeviceCount,omitempty" jsonschema:"Average estimated number of devices or endpoints using the service."`
	ChurnRate          FlexNumber `json:"churnRate,omitempty" jsonschema:"Estimated turnover of IP addresses or endpoints over time, as a fraction between 0 and 1."`
	DistinctASNs       FlexNumber `json:"distinctASNs,omitempty" jsonschema:"Number of unique Autonomous Systems observed for this service."`
	DistinctCountries  FlexNumber `json:"distinctCountries,omitempty" jsonschema:"Number of distinct countries where endpoints for this service reside."`
	DistinctIPs        FlexNumber `json:"distinctIPs,omitempty" jsonschema:"Number of distinct IP addresses observed for this service."`
	DistinctISPs       FlexNumber `json:"distinctISPs,omitempty" jsonschema:"Number of distinct ISPs hosting this service or its endpoints."`
}

// Status is the object returned by GET /status.
type Status struct {
	Active           bool   `json:"active" jsonschema:"Whether the API token is active."`
	QueriesRemaining int    `json:"queriesRemaining" jsonschema:"Queries remaining for the current billing period."`
	ServiceTier      string `json:"serviceTier" jsonschema:"Service tier, e.g., online."`
}

// FeedMetadata is the document returned by GET /v2/{feed}/latest on the feeds
// host. It points at the current feed artifact(s) and carries their generation
// metadata. The JSON container is always present; MMDB is present only for feed
// types also offered in MaxMind DB format.
//
// Type is not part of the upstream document — the client sets it to the feed
// type that was requested so the value is self-describing in JSON/CSV output
// and in the styled report.
type FeedMetadata struct {
	Type string    `json:"feedType,omitempty" jsonschema:"The feed type this metadata describes, e.g. anonymous."`
	JSON FeedFile  `json:"json" jsonschema:"Metadata for the newline-delimited JSON (gzip) feed file."`
	MMDB *FeedFile `json:"mmdb,omitempty" jsonschema:"Metadata for the MaxMind DB feed file, present only for feeds offered in that format."`
}

// FeedFile describes one downloadable feed artifact: where it lives relative to
// the feed root and when it was produced. GeneratedAt, AvailableAt, and Filesize
// are absent on the realtime feed, which carries only Date and Location, so they
// are optional.
type FeedFile struct {
	Date        string `json:"date" jsonschema:"The feed's date. YYYYMMDD for daily feeds; an ISO8601 timestamp for the realtime feed."`
	Location    string `json:"location" jsonschema:"Path to the feed file relative to the feed root, e.g. 20250324/feed.json.gz."`
	GeneratedAt string `json:"generated_at,omitempty" jsonschema:"When the feed was generated, in ISO8601 format."`
	AvailableAt string `json:"available_at,omitempty" jsonschema:"When the feed was made available, in ISO8601 format."`
	Filesize    int64  `json:"filesize,omitempty" jsonschema:"Size of the feed file in bytes."`
}

// Meta carries the response headers that accompany a Context API call. It is
// returned alongside the body so MCP tools can surface quota and result-date
// information.
type Meta struct {
	QueriesRemaining int    `json:"queriesRemaining,omitempty" jsonschema:"Queries remaining in the current billing cycle, from the x-balance-remaining response header."`
	ResultDate       string `json:"resultDate,omitempty" jsonschema:"The date the response is sourced from, in YYYYmmdd format, from the x-result-dt response header."`
}
