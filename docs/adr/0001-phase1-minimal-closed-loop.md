# ADR 0001: Phase 1 minimal closed loop

## Status
Accepted

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
4. Token-based temporary identity flow for MVP (mTLS reserved for next milestone)

## Consequences
- Pros:
  - Fast validation of core control/data flow boundaries
  - Clear interface contracts between three components
  - Low setup complexity for local dev/staging validation
- Cons:
  - Security posture is below target architecture without mTLS + CA lifecycle
  - MASQUE behavior is simulated, not full QUIC/HTTP3 tunnel yet

## Next ADRs
- ADR 0002: mTLS certificate lifecycle and revocation (CRL)
- ADR 0003: control-plane to server interface hardening (gRPC and mTLS)
- ADR 0004: routing/DNS policy enforcement on Linux client
