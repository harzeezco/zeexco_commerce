# 06 Decisions

This section stores important product and engineering decisions.

These are called **ADRs — Architecture Decision Records**, although some of them also cover major product decisions.

The purpose is to preserve:

- what decision was made
- why it was made
- what evidence influenced it
- alternatives considered
- trade-offs
- consequences
- conditions that could cause us to revisit it

A decision should not live only inside:

- the founder's memory
- Slack
- WhatsApp
- a developer conversation
- a code comment

If a decision materially affects how the product is built or operated, it should be documented here.

---

# ADR Format

Every future decision should follow this structure:

## Title

Clear decision name.

## Status

One of:

```
PROPOSED
ACCEPTED
REPLACED
DEPRECATED
```

## Context

What problem are we trying to solve?

## Evidence

What did:

- merchant research
- customer research
- competitor research
- technical constraints

teach us?

## Options Considered

What alternatives existed?

## Decision

What did we choose?

## Why

Why was this option preferred?

## Consequences

What becomes easier or harder because of this decision?

## Revisit When

What evidence could cause this decision to change?

---

# ADR-001 — Use WhatsApp Business Coexistence for Customer Commerce

## Status

ACCEPTED

---

## Context

Our target merchants already use WhatsApp for:

- product enquiries
- referrals
- customer support
- negotiation
- product sharing
- repeat-customer relationships

Many customers already know the merchant through an existing phone number.

Requiring merchants to move customers into a completely new application would introduce additional friction.

---

## Evidence

Merchant research showed that WhatsApp already plays an active role in commerce.

Some merchants:

- post products on WhatsApp Status
- receive customer enquiries
- talk to referred customers
- negotiate through WhatsApp
- maintain long-standing customer relationships through their existing number

Sowo demonstrated a Meta-supported coexistence model where:

- merchant keeps existing WhatsApp Business number
- merchant continues using the mobile app
- customers continue messaging the same number
- platform receives supported API access

Xara demonstrated a different architecture where customer messages may originate from a central platform-owned bot.

That architecture introduces an additional trust question:

> “Who is this platform messaging me?”
> 

---

## Options Considered

### Option A — Central platform WhatsApp bot

Customers interact with our platform-owned WhatsApp number.

### Option B — New merchant phone number

Merchant creates a separate WhatsApp number specifically for our platform.

### Option C — Existing merchant WhatsApp through coexistence

Merchant keeps their existing WhatsApp Business identity while our platform connects through Meta.

---

## Decision

Use:

> **merchant's existing WhatsApp Business identity through supported coexistence infrastructure**
> 

as the primary customer-facing channel.

---

## Why

This preserves:

- customer trust
- existing merchant identity
- existing relationships
- existing contact behaviour
- merchant familiarity with WhatsApp

It reduces behavioural change.

Our product should improve the merchant's existing commerce rather than replace it.

---

## Consequences

### Positive

- lower customer friction
- merchant keeps brand identity
- merchant can manually intervene
- existing relationships remain intact
- easier adoption for WhatsApp-heavy merchants

### Negative

- dependency on Meta
- Meta messaging policies
- messaging costs
- account connection complexity
- integration failure states
- provider/API changes

---

## Revisit When

Revisit if:

- Meta materially changes coexistence access
- pricing makes model commercially unsustainable
- merchants strongly prefer a separate business channel
- another channel becomes dominant

---

# ADR-002 — Build Commerce Before Payments

## Status

ACCEPTED

---

## Context

The broader company opportunity includes:

- merchant sales
- customer management
- domestic payments
- supplier payments
- cross-border payments

However, merchant research suggests that immediately asking merchants to trust a new platform with money movement creates a higher adoption barrier.

Merchants already have existing payment habits.

---

## Evidence

Research showed:

- merchants are guarded around financial information
- some merchants already have payment methods they trust
- one merchant was comfortable converting NGN to GBP and paying suppliers through their existing process
- merchants were more willing to discuss sales problems than financial infrastructure
- merchants already experience problems around customer follow-up, product presentation and sales conversion

---

## Options Considered

### Option A

Launch directly as a cross-border payment product.

### Option B

Build payments and commerce simultaneously.

### Option C

Start with commerce value, establish trust and usage, then introduce payments.

---

## Decision

Use the sequence:

```
Commerce
↓
Payments
↓
Broader Business Operations
```

---

## Why

Commerce provides a lower-risk entry point.

If merchants already rely on the product for:

- customers
- products
- conversations
- orders
- follow-up

then payments become a natural extension.

Example:

Instead of selling:

> “Use our payment service.”
> 

later we can say:

> “This customer has an unpaid order. Would you like to collect the payment here?”
> 

Context significantly improves payment adoption.

---

## Consequences

### Positive

- lower initial regulatory burden
- faster MVP
- easier merchant trust building
- learn merchant workflow before handling money
- payment features later gain richer commerce context

### Negative

- payments revenue delayed
- another provider may solve financial side first
- commerce product must provide enough standalone value

---

## Revisit When

Revisit if research demonstrates that:

- payments are the dominant pain
- merchants will adopt financial infrastructure without prior commerce relationship
- a partnership significantly reduces regulatory/product complexity

---

# ADR-003 — AI Is Not the Source of Truth

## Status

ACCEPTED

---

## Context

The product will use AI heavily for:

- customer conversations
- merchant summaries
- sales assistance
- product descriptions
- intent detection
- follow-up
- negotiation assistance

However, commerce contains important facts and rules.

Examples:

- product price
- minimum acceptable price
- inventory status
- order status
- payment status
- permissions

These cannot depend on probabilistic model output.

---

## Options Considered

### Option A

Put business information inside prompts and allow AI to reason directly.

### Option B

Give AI direct database access.

### Option C

Store business state deterministically and expose controlled tools to AI.

---

## Decision

Use:

> **structured systems as source of truth + AI as reasoning/orchestration layer**
> 

---

## Architecture

```
Customer message
↓
AI understands intent
↓
AI requests business information through tools
↓
Deterministic application returns truth
↓
AI communicates result
```

Example:

Customer:

> “How much is the G-Shock?”
> 

AI does not remember the price from a previous conversation.

It calls:

```
get_product()
get_pricing_policy()
```

---

## Why

This reduces:

- hallucination
- unauthorized discounts
- stale information
- accidental financial actions
- security risk

---

## Consequences

### Positive

- safer architecture
- easier auditing
- business rules testable without AI
- model providers can change
- better consistency

### Negative

- more engineering work
- tools must be carefully designed
- requires structured data
- agent orchestration becomes more complex

---

## Revisit When

The principle itself should rarely change.

Individual AI capabilities can expand as models improve, but authority over business state should remain deterministic.

---

# ADR-004 — Human Takeover Is a First-Class Conversation State

## Status

ACCEPTED

---

## Context

A sales AI cannot safely handle every interaction indefinitely.

Merchants must remain able to:

- intervene
- answer personally
- approve exceptions
- resolve sensitive situations

If AI and merchant respond simultaneously, the customer experience becomes confusing.

---

## Evidence

Sowo demonstrated:

> human reply → AI paused
> 

Merchant research also indicates that merchants want control over:

- pricing
- customer relationships
- sensitive decisions

---

## Options Considered

### Option A

AI continues unless explicitly disabled globally.

### Option B

AI detects human messages informally from conversation history.

### Option C

Conversation ownership becomes explicit application state.

---

## Decision

Implement explicit states such as:

```
AI_HANDLING
WAITING_FOR_MERCHANT
HUMAN_HANDLING
PAUSED
ESCALATED
CLOSED
```

---

## Rule

When merchant/staff replies:

```
HUMAN_MESSAGE_SENT
↓
AI pauses
↓
conversation owner = HUMAN
```

AI can only resume according to deterministic policy.

---

## Sensitive Conversations

Examples:

- complaint
- refund
- fraud
- damaged product
- payment dispute

may require explicit human release before AI resumes.

---

## Consequences

### Positive

- predictable customer experience
- merchant remains in control
- safer escalation
- clear engineering behaviour

### Negative

- requires synchronization between app and WhatsApp
- takeover/resume logic adds state complexity
- timers need reliable background processing

---

# ADR-005 — Use a Modular Monolith for the MVP

## Status

ACCEPTED

---

## Context

The product contains several domains:

- businesses
- WhatsApp integrations
- customers
- conversations
- products
- pricing
- orders
- follow-ups
- agents
- automation

It would be possible to create independent microservices immediately.

However, the product is still being validated.

---

## Options Considered

### Option A

Microservices from the beginning.

### Option B

Unstructured monolith.

### Option C

Modular monolith with explicit domain boundaries.

---

## Decision

Build a:

> **modular monolith**
> 

for the MVP.

Example:

```
app/
├── businesses/
├── integrations/
├── customers/
├── conversations/
├── products/
├── pricing/
├── orders/
├── followups/
├── automation/
├── agents/
└── audit/
```

---

## Why

The current priority is:

- learning
- speed
- reliability
- simplicity

not independent service scaling.

A modular monolith provides clear separation without creating premature distributed-system complexity.

---

## Consequences

### Positive

- faster development
- easier local setup
- simpler deployment
- easier transactions
- easier debugging
- suitable for junior developers joining team

### Negative

- discipline required to maintain module boundaries
- careless implementation can become tightly coupled
- individual domains may eventually need extraction

---

## Revisit When

Consider service extraction when there is evidence such as:

- materially different scaling requirements
- independent deployment needs
- reliability isolation
- large engineering teams
- performance bottlenecks

Do not migrate because “microservices are more scalable” in theory.

---

# ADR-006 — Merchant Assistant and Customer Commerce Agent Are Separate Experiences

## Status

ACCEPTED

---

## Context

The product contains two fundamentally different conversations.

### Customer conversation

Customer talks to merchant.

Purpose:

> sell.
> 

### Merchant conversation

Merchant talks to our platform.

Purpose:

> operate and understand business.
> 

Combining them into the same conceptual bot creates confusion.

---

## Decision

Create two distinct experiences.

### Commerce Agent

Customer-facing.

Operates through merchant's connected WhatsApp.

Responsible for:

- product enquiries
- sales
- permitted negotiation
- follow-up
- order creation

### Merchant Assistant

Merchant-facing.

Responsible for:

- business summaries
- customer queries
- product management
- follow-up actions
- operational commands

Later:

- payments
- supplier transfers
- reconciliation

---

## Shared Infrastructure

Both agents may use the same underlying:

- product data
- customer data
- orders
- business rules
- commerce services

but have different:

- permissions
- tools
- prompts
- interaction policies

---

## Why

Customer and merchant have very different goals.

Separating them improves:

- security
- tool permissions
- context
- UX
- reasoning quality

---

# ADR-007 — Dashboard Is Secondary to Natural-Language Operations

## Status

ACCEPTED

---

## Context

Competitors commonly provide dashboards containing:

- revenue charts
- conversion metrics
- customer lists
- AI activity
- order analytics

These are useful but may not match how many physical merchants work.

---

## Evidence

Merchant research repeatedly showed:

> merchants are busy.
> 

Our target merchant should not need to become an analyst.

---

## Decision

Dashboard remains available as:

- system of record
- product management interface
- conversation inspection
- settings
- advanced management

But the main merchant experience should emphasize:

- WhatsApp
- notifications
- natural-language summaries
- actionable recommendations

---

## Example

Do not prioritize:

> Conversion rate: 18%.
> 

Prefer:

> “11 customers asked about products today. 3 bought and 4 still need follow-up.”
> 

---

## Consequences

This requires strong internal analytics even if we hide the complexity externally.

Natural-language simplicity should not mean lack of structured data.

---

# ADR-008 — Automatically Build Customer Memory

## Status

ACCEPTED

---

## Context

Traditional CRM tools often require:

- manual contact creation
- manual notes
- manual activity updates
- pipeline maintenance

Our merchants may not consistently perform this administrative work.

---

## Decision

Customer records should emerge automatically from commerce activity.

Example:

```
Customer sends first message
↓
Customer created

Customer asks about product
↓
Interest recorded

Customer orders
↓
Purchase history updated

Customer returns
↓
Returning customer recognized
```

---

## Merchant Role

Merchant may add:

- notes
- labels
- corrections

but should not be responsible for creating the entire customer history.

---

## Why

The product should:

> learn the merchant's business
> 

rather than ask the merchant to continually teach software what happened.

---

# ADR-009 — Follow-Up Is a Core Commerce Capability

## Status

ACCEPTED

---

## Context

A repeated research pattern is:

> customer asks for price → customer stops responding.
> 

This occurred across merchants using different selling channels.

---

## Decision

Follow-up is part of MVP rather than a later marketing feature.

The system should eventually understand events such as:

```
PRICE_SENT
↓
NO_RESPONSE
↓
FOLLOWUP_DUE
```

---

## Important Constraint

Do not build blind automated spam.

Follow-up decisions should consider:

- context
- previous attempts
- product availability
- customer status
- messaging rules
- consent
- merchant policy

---

## Success Measurement

Track:

- follow-ups sent
- replies recovered
- recovered sales
- recovered revenue

This provides a direct way to demonstrate product value.

---

# ADR-010 — Negotiation Rules Must Be Deterministic

## Status

ACCEPTED

---

## Context

Price negotiation is common in local commerce.

We want AI to participate without giving away merchant margin.

---

## Decision

Maintain structured rules such as:

```
asking_price
minimum_price
negotiation_enabled
approval_threshold
```

AI may decide how to communicate within these limits.

AI may not modify the limits.

---

## Example

```
asking price = ₦23,000
minimum price = ₦20,000
```

If customer offers:

```
₦18,000
```

AI cannot accept.

It either:

- counters within allowed rules
- asks merchant
- declines

---

# ADR-011 — Product Creation Should Begin From Merchant Language and Images

## Status

ACCEPTED

---

## Context

Competitor product forms often require many fields.

Examples:

- name
- description
- category
- variants
- stock
- content ID
- bulk pricing

This creates work for merchants.

---

## Decision

Default product onboarding should begin with:

> **photo + natural-language merchant description**
> 

Example:

> “LG 10kg, 280k, last 260k.”
> 

The system converts that into structured data.

---

## Why

Merchant already knows the product.

Our job is to structure what they know, not force them to become data-entry staff.

---

## Consequences

AI extraction must:

- allow confirmation
- expose uncertainty
- avoid inventing attributes
- preserve original input

---

# ADR-012 — Preserve Truth in Product Images

## Status

ACCEPTED

---

## Context

AI image enhancement can make products easier to sell.

However, misleading enhancement can destroy customer trust.

Especially important for:

- used appliances
- electronics
- physical goods with condition differences

---

## Decision

AI may improve presentation but may not materially misrepresent the product.

Allowed:

- lighting
- crop
- background cleanup
- clarity

Not allowed:

- removing real defects
- changing model
- fabricating accessories
- making used product appear new
- changing important visible characteristics

---

## Why

Trust is more important than short-term visual attractiveness.

---

# ADR-013 — External Integration State Must Be Recoverable

## Status

ACCEPTED

---

## Context

During Sowo research, Meta showed the WhatsApp account as connected while Sowo temporarily continued showing an unconnected/error state.

This demonstrates a classic distributed-system problem:

> external success + internal failure
> 

---

## Decision

External provider state and internal state must be stored separately.

Example:

```
meta_status = CONNECTED
internal_status = SYNC_FAILED
```

The system must support reconciliation.

---

## Rule

Never force merchants to repeat a successful external authorization merely because our internal callback failed.

---

## Required Mechanisms

- idempotency
- external event identifiers
- reconciliation workers
- retries
- visible status
- recoverable failure states

---

# ADR-014 — Physical and Digital Commerce Belong to the Same Product

## Status

ACCEPTED

---

## Context

Our target businesses are not necessarily online-only merchants.

A single merchant may receive sales through:

- walk-ins
- calls
- WhatsApp
- referrals
- Facebook
- Jiji
- physical market
- warehouse

---

## Decision

Do not design the domain model as:

> ecommerce-only.
> 

The underlying concepts should represent commerce independent of channel.

Example:

```
Customer
Order
Product
Payment
```

should not require that the sale originated through WhatsApp.

---

## Why

The merchant's business is one business.

Our system should eventually connect its channels rather than creating separate worlds.

---

# ADR-015 — Build Locally, Keep Core Architecture Geography-Neutral

## Status

ACCEPTED

---

## Context

Initial research and pilots are in Nigeria.

The long-term vision includes:

- other cities
- states
- countries

---

## Decision

Initial product experience may be Nigeria-focused.

Core domain architecture should not hardcode assumptions such as:

```
currency = NGN
country = Nigeria
payment_provider = one provider
```

These should be business/configuration attributes.

---

## Examples

```
Business.country
Business.currency
Business.timezone
```

Future integrations can vary by market.

---

# ADR-016 — Events Represent Important Business State Changes

## Status

ACCEPTED

---

## Context

Many product actions trigger future actions.

Example:

```
PRICE_SENT
```

may later result in:

```
FOLLOWUP_DUE
```

An order being delivered may later trigger:

```
CUSTOMER_RATING_REQUESTED
```

---

## Decision

Meaningful domain changes should produce explicit events.

Examples:

```
MESSAGE_RECEIVED
PRODUCT_INTEREST_DETECTED
PRICE_SENT
HUMAN_TAKEOVER
FOLLOWUP_DUE
ORDER_CREATED
ORDER_DELIVERED
```

---

## Why

Events help separate:

- immediate action
- future automation
- analytics
- auditing

without tightly coupling every feature.

---

# ADR-017 — Initial Pilot Before Broad Launch

## Status

ACCEPTED

---

## Context

The product contains several assumptions that are not yet sufficiently validated.

Examples:

- merchants will allow AI to talk to customers
- follow-up will improve conversion
- merchants will trust product automation
- merchants will continue using the system
- merchants will eventually pay

---

## Decision

Launch first with approximately:

> **3–4 design-partner merchants**
> 

rather than broad public acquisition.

---

## Pilot Relationship

Free or subsidized use should come with an explicit exchange:

merchant receives early access.

We receive:

- regular usage
- feedback
- permission to observe workflows
- product improvement conversations

---

## What We Measure

For each pilot merchant:

```
enquiries
AI-handled conversations
human takeovers
follow-ups
customer replies
orders created
sales recovered
revenue influenced
merchant time saved
retention
```

---

## Exit Criteria

Move beyond pilot when there is evidence that:

- merchants repeatedly use it
- merchant work is reduced
- sales outcomes improve
- product is reliable
- value is understandable
- some merchants are willing to pay

---

# ADR-018 — Messaging and AI Costs Must Be Part of Unit Economics

## Status

ACCEPTED

---

## Context

The product incurs variable costs through:

- WhatsApp messaging
- AI inference
- image generation
- storage
- payment providers later

An “unlimited AI messages” model could become economically dangerous.

---

## Decision

Product pricing must eventually account for:

```
subscription revenue
-
WhatsApp cost
-
AI cost
-
image processing cost
-
infrastructure
-
support
```

Follow-up and broadcast features must be cost-aware.

---

## Consequences

Future plans may use:

- included quotas
- usage tiers
- credits
- fair-use limits
- overage pricing

rather than unlimited variable-cost actions without safeguards.

---

# Decision Register

Current accepted decisions:

```
ADR-001 WhatsApp coexistence
ADR-002 Commerce before payments
ADR-003 AI is not source of truth
ADR-004 Human takeover model
ADR-005 Modular monolith
ADR-006 Separate Merchant Assistant and Commerce Agent
ADR-007 Dashboard secondary
ADR-008 Automatic customer memory
ADR-009 Follow-up is core
ADR-010 Deterministic negotiation
ADR-011 Natural-language product creation
ADR-012 Truth-preserving image enhancement
ADR-013 Recoverable integration state
ADR-014 Physical + digital commerce
ADR-015 Geography-neutral architecture
ADR-016 Domain events
ADR-017 Pilot before scale
ADR-018 Cost-aware AI/messaging
```

This register should grow as the company makes consequential decisions.

---

# Decision Review Rule

Do not treat an ADR as permanent just because it exists.

When new evidence contradicts a decision:

1. do not silently change implementation
2. create a new ADR
3. reference the previous ADR
4. explain what changed
5. mark the old decision as `REPLACED`

Example:

```
ADR-022 replaces ADR-005
```

This gives future employees the history of the product rather than only its current state.

---

# Why This Section Matters

A new engineer should be able to read this section and understand:

> why WhatsApp matters
> 

> why payments are not first
> 

> why AI cannot directly control business rules
> 

> why merchants can take over conversations
> 

> why we are not building microservices immediately
> 

> why our dashboard is intentionally secondary
> 

> why follow-up exists in the MVP
> 

> why the system supports physical commerce
> 

without needing the founder to explain every decision personally.

That is how the company begins turning founder knowledge into institutional knowledge.