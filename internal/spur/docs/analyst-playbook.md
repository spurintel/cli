# Spur Analyst Playbook

## "What does Spur think about this IP?"

Follow this procedure for any IP assessment question.

### Step 1: Query the IP
Call `get_ip_context(ip)`. Pass `dt=YYYYmmdd` if the user specified a date.

### Step 2: Identify operators to look up
Collect every service tag from the response:
- Every `tunnels[].operator` (e.g. `NEXX_PROXY`, `PROTON_VPN`).
- Every `client.proxies[]` entry.
- Every `client.behaviors[]` value matching `^[A-Z0-9_]+_USER$` — strip the `_USER` suffix to recover the tag.

### Step 3: Fan out to tag metadata
For EACH tag from Step 2, call `get_tag_metadata(tag)`. Do them in parallel. Do not skip any.

### Step 4: Render via `format_assessment`
Call `format_assessment` with:
- `context` — the raw `get_ip_context` response.
- `tags` — a map from tag name to its `get_tag_metadata` response.
- `bottom_line` — one sentence classifying the IP plus risk framing when warranted. Two clauses, max.
- `assessment` — a plain-English paragraph explaining what the findings mean for the customer in practice.

The tool renders the final six-section markdown deterministically and validates your prose against fidelity rules. Send the returned markdown verbatim.

## Field primer — commonly confused fields

### `tunnels[]` vs `client.proxies[]` — fundamentally different signals

- **`tunnels[]`** — *"this IP **is** an exit node for this service."* Each entry means Spur attributes the IP itself to that VPN / Tor / proxy infrastructure. Use these when saying *"this IP is a Proton VPN exit."*
- **`client.proxies[]`** — *"clients observed connecting **from** this IP have been using these proxy services."* Does **not** mean the IP is one of those proxies. Typical case: a residential IP whose owner installed a proxy-reseller SDK.

Never conflate them. If asked *"is this IP a proxy?"* the answer lives in `tunnels[]`. If asked *"what are the clients on this IP doing?"* the answer lives in `client.proxies[]` and `client.behaviors[]`. Both warrant a `get_tag_metadata` lookup, but the narrative role is different.

`client.behaviors[]` values matching `^*_USER$` (e.g. `TOR_PROXY_USER`) mean *"a user of that service was observed on this IP,"* not *"this IP is that service."*

### Unattributed and partial tunnels

A `tunnels[]` entry with `operator` empty (or only `type` set — `VPN`, `PROXY`, `REMOTE_DESKTOP`) still means *the IP is a tunnel exit*; Spur just can't name the service. Common for home-hosted OpenVPN, enterprise appliances (SonicWall, Cisco ASA), and unbranded SSTP. Don't paraphrase as a risk enum.

Sanctioned phrasings for prose: *"tunnel with unattributed operator"*, *"home-hosted VPN,"* *"unbranded VPN appliance."* Confidence should be framed lower (no `get_tag_metadata` lookup exists for these entries) and `services[]` (e.g. `OPENVPN`, `IPSEC`, `WIREGUARD`) is the corroborating signal when present.

### `client.concentration` — where the CLIENTS are, not where the IP is

`concentration` describes where the clients using this IP cluster physically. The IP's own registered location lives at the top-level `location` field. These are different things.

- `city` / `state` / `country` — the *center* of the client base, not the IP's location. Compare with `location` to spot skew.
- `density` (0–1) — how tightly clustered the clients are around that center. **Not volume.** Volume is `client.count` + `client.spread`. Never render as a percentage.
- `skew` (kilometers) — distance from the IP's `location` to the concentration center. **High skew is a strong tunnel/proxy signal** — the IP is registered in one place but clients using it are far away. This is **not** the same as the `GEO_MISMATCH` risk enum; do not rename it.
- `countries` — distinct client countries. Many is suspicious for a single residential/mobile IP.

### `client.types` and `client.spread`

- `client.types` — device mix (`MOBILE`, `DESKTOP`, `HEADLESS`, `IOT`). `HEADLESS` on a VPN exit reads as scripted/automation; `MOBILE`-dominant on a residential IP simply reads as mobile device identifiers observed behind a residential IP; `DESKTOP` + high `client.count` reads as a gateway / shared NAT / botnet.
- `client.spread` (km) — geographic *dispersion* of the client base; pair with `density`. High density + low spread = tight residential cluster. Low density + high spread = SDK installations scattered across regions. High density + high spread = a large coordinated proxy pool.

### `services[]`

Protocol-level detections on the IP itself (`OPENVPN`, `IPSEC`, `WIREGUARD`, `SSH`, etc.). Corroborates a tunnel when present; absence does **not** negate a tunnel (some attributions come from passive DNS or peer scanning, not open-port detection).

## Data freshness

Different fields age off at different cadences. Relevant when the user asks about a past date (`dt=`) or wonders why a label "went away":

- Tunnels / VPN attributions: ~14–21 days.
- Residential-proxy / SDK client signals: ~3–7 days.
- Location and AS: ~30 days, but rarely changes.

## Common confusions worth naming

- **Mobile carriers / ISP gateways.** European and APAC telco IPs often present `MOBILE_PROXY`-adjacent signals without `anonymous=true`. Carrier NAT and traffic-optimization proxies are not anonymizing — assess on `tunnels[].anonymous` and `isAnonymous`, not on the mere presence of a proxy tag.
- **Enterprise ZTNA / SASE (Zscaler, Cloudflare WARP, Netskope, Fortinet, Palo Alto Prisma).** Legitimate corporate egress. Category is `ZTNA_PROVIDER` and tags are typically non-anonymous. Do not escalate to "elevated scrutiny" on category alone; the risk depends on user intent, not IP classification.
- **Tor.** Spur reports `TOR_PROXY` (IP is an exit) and `TOR_PROXY_USER` (a Tor user was observed on this IP). Relay vs. exit is not distinguished beyond the fact that an observable exit is what customers see.

## Risk framing

- **Anonymous operator** (`isAnonymous: true` on a tag OR `anonymous: true` on a tunnel entry): flag as **high risk**. Traffic cannot be attributed to an end user. Recommend elevated scrutiny / step-up auth / gating for high-value flows. `format_assessment` will auto-select the recommendation line.
- **Callback / malware proxy** (`risks` contains `CALLBACK_PROXY` OR any tag has `isCallbackProxy: true`): flag with stronger language. Commonly associated with malware-operated botnet traffic. Recommend blocking / quarantine.
- **No-log** (`isNoLog: true`): aggravating factor when combined with anonymity — forensic attribution after the fact is not possible.
- **Inactive** (`isInactive: true`): downgrade the concern; Spur no longer tracks active endpoints.

Lead `bottom_line` with the strongest of these framings that applies.

## Fidelity rules (enforced by `format_assessment`)

The tool will reject your call with an error if:
- `bottom_line` or `assessment` cites a risk enum (`AD_FRAUD`, `CALLBACK_PROXY`, `GEO_MISMATCH`, `LOGIN_BRUTEFORCE`, `TUNNEL`, `WEB_SCRAPING`) — or a common paraphrase like *"geo mismatch"* or *"ad fraud"* — that is **not** present in `context.risks[]`. High skew alone is not `GEO_MISMATCH`.
- `bottom_line` or `assessment` converts `density` to a percentage.

On rejection, revise your prose and retry. Do not work around the validator by rephrasing to hide the intent — if a use-case isn't backed by a Spur-returned enum, drop the claim entirely.

## Response codes and recovery
See `spur://docs/context-api-endpoints`. 401/403 → surface with sales link. 429 → call `get_api_status`.
