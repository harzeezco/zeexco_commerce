# Zeexco Core

Zeexco is a commerce operating platform for merchants who sell through
physical stores, WhatsApp, referrals, and online channels.

## Repository Responsibility

This repository contains the deterministic Go commerce backend.

It owns:
- businesses
- products
- pricing policies
- customers
- conversations
- orders
- follow-ups
- WhatsApp integration
- authorization
- domain events

It does NOT own:
- LLM orchestration
- prompts
- model routing
- AI provider credentials

Those belong to the separate Zeexco Agent Platform.

## Architecture Rules

1. PostgreSQL is the commerce source of truth.
2. Kafka carries asynchronous domain events.
3. The AI platform never writes directly to commerce tables.
4. Business rules must remain deterministic.
5. WhatsApp webhook handling must persist and acknowledge quickly.
6. AI workflows run asynchronously after events are published.
7. Every tenant-owned record must be scoped by business_id.
8. Important actions must be auditable.

## Required Reading Before Major Changes

Read:

- docs/product/product-overview.md
- docs/product/product-principles.md
- docs/architecture/system-architecture.md
- docs/architecture/domain-model.md
- docs/decisions/

For work on a specific flow, read the corresponding document in docs/flows/.

## Current Implementation Phase

We are building the deterministic commerce foundation first.

Current priority:

Business
→ Product
→ PricingPolicy
→ PostgreSQL
→ Transactional Outbox
→ Kafka

Do not implement the Agent Platform in this repository.