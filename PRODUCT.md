# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Stack

Delegated: Go backend serving an embedded React + TypeScript interface. The choice favors a small cross-platform executable, reliable process and port management, and a browser UI at `http://localhost:4747`.

## Users

Self-hosting hobbyists and small homelab operators who run websites or services locally and want to publish selected ports without router port-forwarding.

## Product Purpose

Make Cloudflare Tunnel approachable as a local operator tool: connect a Cloudflare account, discover or enter a local service, choose a hostname, and start or stop its secure tunnel from one interface.

## Positioning

The product turns the machine's currently listening services into an actionable publishing workflow instead of requiring users to hand-author tunnel configuration and DNS records.

## Operating Context

The application runs locally on Windows, macOS, and Linux. Its interface is served on port 4747. It manages the official `cloudflared` client, opens Cloudflare's browser-based login flow, and presents tunnel state and logs locally.

## Capabilities and Constraints

- Discover listening TCP ports and allow manual service entry.
- Prioritize likely HTTP/HTTPS services, with search and an optional all-ports view.
- Associate a local HTTP service with a user-selected hostname.
- Detect the domain authorized by `cloudflared tunnel login`, accept only the subdomain in the common flow, and check existing DNS records before publishing.
- Start, stop, inspect, and persist tunnel definitions.
- Use Cloudflare Tunnel rather than router port-forwarding.
- Never store Cloudflare credentials in source control or expose the management UI publicly by default.
- Initial release depends on the official `cloudflared` executable being installed or supplied alongside the application.
- Distribution targets are Windows executable/installer, macOS application/DMG, and Linux binary/packages.
- Automatic named-tunnel DNS creation requires an authenticated Cloudflare account and a domain managed in that account.

## Brand Commitments

Product name: Portivane. Product language should be practical, friendly, and understandable without networking expertise.

## Evidence on Hand

No testimonials, benchmarks, customer logos, or finalized brand assets exist. Future surfaces must not fabricate them.

## Product Principles

- Local first: management stays bound to the user's own machine by default.
- Show the route: always make the local service, public hostname, and tunnel state legible together.
- Safe defaults: tunneling replaces open inbound router ports, secrets stay outside application data, and destructive actions require deliberate confirmation.
- Progressive depth: the common publish flow is simple while logs and configuration remain available when troubleshooting.
- Portable ownership: configurations can move between Windows, macOS, and Linux without changing the mental model.

## Accessibility & Inclusion

The web interface should meet WCAG 2.2 AA expectations, support keyboard operation, visible focus, reduced motion, and responsive layouts.
