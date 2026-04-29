# ADR 0001: Phase 1 minimal closed loop

## Status
Accepted (amended: token-only device identity; no device mTLS track)

## Context
The project starts from an empty repository. Requirement document recommends implementing one stable end-to-end closed loop first:

- register user
- issue activation material
- client connects
- server authorizes
- audit is visible

## Decision
For initial delivery, we implement:

1. Control plane in Laravel (REST JSON, `/api/v1`)
2. Server in Go with `POST /connect` + `/healthz`
3. Linux CLI in Go with `activate/connect/status/disconnect`
4. **Token-based device identity** (Bearer / JWT-style) with control-plane authorize callbacks — **not** mTLS or client certificates

## Consequences
- Pros:
  - Fast validation of core control/data flow boundaries
  - Clear interface contracts between three components
  - Low setup complexity for local dev/staging validation
  - Aligns with `开发需求.md` §2.3: **no org private CA for device identity**, **HTTPS via ACME (or equivalent public trust)** for operator-facing surfaces
- Cons:
  - Transport-level hardening between control-plane and masque remains HTTPS-centric until a separate hardening milestone
  - MASQUE behavior was stubbed first; full QUIC/HTTP3 production paths evolve incrementally

## Follow-up ADRs (optional / superseded items)
- ~~ADR 0002: mTLS certificate lifecycle~~ — **withdrawn**: device mTLS / internal CA issuance not in scope
- ADR 0003 (future): control-plane ↔ masque interface hardening (**gRPC or mTLS is not required**; prefer **mutual auth patterns compatible with token authorize**, over **publicly trusted TLS**)

## Related
- `开发需求.md` §2.3 non-goals (no device certs, no IPv6 dataplane short-term, ACME TLS)
