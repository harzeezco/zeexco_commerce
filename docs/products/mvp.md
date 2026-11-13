# PRD-001 — Commerce MVP

## Document Status

**Status:** Draft v0.1

**Product:** Merchant Commerce Platform

**Phase:** Commerce MVP

**Owner:** Product

**Related Knowledge Base:**

- 01 Company
- 02 Research
- 03 Product
- 04 User Flows
- 05 Engineering
- 06 Decisions

---

# 1. Product Summary

We are building an intelligent commerce operating layer for merchants who sell through a combination of:

- physical shops
- warehouses
- WhatsApp
- referrals
- social media
- phone calls
- other informal commerce channels

The Commerce MVP focuses on helping merchants:

- respond to customers faster
- present products better
- remember customer context
- follow up with interested customers
- negotiate within merchant-defined rules
- convert conversations into structured orders
- reduce repetitive sales work

The product should work around the merchant's existing business rather than requiring the merchant to move their entire business into new software.

---

# 2. Problem

Many merchants already have products and customers.

Their main problem is not simply lack of access to ecommerce.

Merchant research shows recurring operational problems:

- merchants are busy
- customer conversations are spread across WhatsApp, calls and physical interactions
- customers frequently ask for price and disappear
- merchants do not consistently follow up
- customer history is buried inside chats
- product presentation is inconsistent
- online selling requires additional work
- merchants may forget interested or previous customers
- digital tools often require too much setup or data entry

As a result, sales opportunities are lost even when demand already exists.

---

# 3. Product Hypothesis

If we help merchants manage customer conversations, product information, follow-up and order creation through the channels they already use, then merchants will:

- spend less time on repetitive work
- lose fewer customer opportunities
- convert more enquiries into sales
- maintain better customer relationships
- become more willing to use the platform for deeper business operations later

If merchant trust and daily usage are established, payments can later be introduced naturally into the same commerce workflow.

---

# 4. Target User

## Primary User

A commerce merchant who:

- sells physical products
- has existing customers
- receives customer enquiries
- uses WhatsApp for at least part of the business
- operates physically, digitally, or both
- is often too busy to manage every customer interaction
- may depend on referrals and repeat customers
- does not want complicated software

Examples include:

- appliance sellers
- electronics merchants
- fashion merchants
- watch/accessory sellers
- home and living merchants
- general retailers

---

# 5. Jobs to Be Done

## Job 1 — Respond to customers

> When customers contact me while I am busy, help answer the questions that do not require my personal decision.
> 

## Job 2 — Present products

> When I have something to sell, help me turn a basic product photo and price into something ready to send to customers.
> 

## Job 3 — Remember customers

> When customers interact with my business, remember what they asked about so I don't have to.
> 

## Job 4 — Recover missed sales

> When a customer shows interest and disappears, help me know when and how to follow up.
> 

## Job 5 — Protect pricing

> When customers negotiate, help handle the negotiation without going below the price I have approved.
> 

## Job 6 — Create orders

> When a customer agrees to buy, turn the conversation into a proper order without making me re-enter everything manually.
> 

---

# 6. Goals

The Commerce MVP should prove that the platform can:

1. integrate successfully with merchant WhatsApp Business accounts
2. support real merchant/customer conversations
3. use structured product data to answer sales questions
4. automatically create useful customer memory
5. safely support merchant-approved negotiation
6. allow human takeover
7. identify meaningful follow-up opportunities
8. turn confirmed purchase intent into orders
9. reduce merchant workload
10. produce measurable commercial value

---

# 7. Non-Goals

The MVP is not intended to become:

- full ERP software
- accounting software
- full inventory management
- a logistics marketplace
- a POS system
- a new ecommerce marketplace
- a website builder
- an advanced CRM
- a complete financial platform
- a cross-border payment platform

These may be introduced in later phases.

---

# 8. Core Product Experience

The Commerce MVP has two major surfaces.

## 8.1 Customer Commerce Surface

Customer communicates with the merchant's existing WhatsApp Business account.

The platform assists behind the merchant's identity.

Core capabilities:

- customer enquiry
- product discovery
- pricing
- negotiation
- order intent
- escalation
- follow-up

---

## 8.2 Merchant Assistant

Merchant interacts with our system through natural language.

Initial functions may include:

- add product
- review customer follow-ups
- see customers needing attention
- receive daily summary
- query business activity

The merchant assistant is not the same agent as the customer-facing commerce agent.

---

# 9. MVP User Flows

The MVP consists of:

## F001 Merchant Activation

```
signup
→ business setup
→ WhatsApp connection
→ activation
```

## F002 Product Creation

```
photo + merchant description
→ product draft
→ merchant confirmation
→ product active
```

## F003 Sales Conversation

```
customer message
→ conversation
→ product/customer context
→ AI response
```

## F004 Negotiation

```
customer discount request
→ pricing policy
→ approved negotiation / merchant escalation
```

## F005 Human Takeover

```
merchant or system escalation
→ AI paused
→ human controls conversation
```

## F006 Follow-Up

```
customer shows intent
→ stops progressing
→ follow-up identified
→ merchant/automation follows up
```

## F007 Order Creation

```
purchase intent confirmed
→ order data resolved
→ order created
```

---

# 10. Functional Requirements

## FR-001 — Business Workspace

System must allow a merchant to create a business.

A business must own its own:

- products
- customers
- conversations
- orders
- rules
- integrations

---

## FR-002 — WhatsApp Connection

Merchant must be able to connect an eligible WhatsApp Business account through supported Meta infrastructure.

The system must distinguish:

- external Meta connection state
- internal platform provisioning state

---

## FR-003 — Customer Creation

When a previously unknown customer messages a merchant:

- customer record must be created automatically
- customer must belong to the correct business
- first interaction time must be recorded

---

## FR-004 — Conversation Persistence

Messages must be linked to:

- business
- customer
- conversation

Conversation context must persist between customer messages.

---

## FR-005 — Product Creation

Merchant must be able to create a product using:

- image
- natural-language description

The system should derive structured product information where possible.

Merchant must confirm before product becomes active.

---

## FR-006 — Product Retrieval

AI must retrieve product information through controlled tools.

AI must not invent:

- price
- availability
- stock
- product characteristics

---

## FR-007 — Pricing Rules

System must support at minimum:

- asking price
- minimum price
- negotiation enabled/disabled

Merchant-defined minimum price cannot be overridden by AI.

---

## FR-008 — Sales Conversation

AI may automatically respond when:

- conversation state permits it
- required information is available
- action does not require human approval

---

## FR-009 — Escalation

AI must be able to escalate conversations when:

- information is missing
- confidence is insufficient
- customer requests human interaction
- merchant policy requires approval
- sensitive scenario is detected

---

## FR-010 — Human Takeover

When merchant responds manually:

- AI must pause
- conversation ownership must change
- AI must not compete with merchant

---

## FR-011 — Follow-Up

System must support follow-up reasons such as:

- price sent but no reply
- product enquiry abandoned
- customer said they would return
- payment pending
- product back in stock

---

## FR-012 — Order Creation

System must support creation of an order containing:

- merchant/business
- customer
- product
- quantity
- agreed price
- status

Order should be linked to originating conversation where applicable.

---

## FR-013 — Daily Summary

Merchant should be able to receive a business-language summary containing information such as:

- enquiries
- sales/orders
- follow-ups
- customers requiring attention

---

# 11. AI Requirements

AI is used for:

- intent detection
- natural-language understanding
- product description generation
- sales response generation
- merchant summarization
- follow-up wording
- conversation reasoning

AI is not responsible for:

- authorization
- minimum price
- stock truth
- order ownership
- payment status
- permissions

---

# 12. AI Tool Requirements

Initial tools may include:

```
get_business
search_products
get_product
get_pricing_policy
get_customer
get_customer_history
get_conversation
create_order_draft
create_order
schedule_followup
request_merchant_decision
escalate_conversation
send_message
```

Tool calls must be auditable.

---

# 13. Conversation State Requirements

Minimum states:

```
AI_HANDLING
WAITING_FOR_MERCHANT
HUMAN_HANDLING
PAUSED
ESCALATED
CLOSED
```

The application controls state transitions.

The model does not independently assign itself conversation authority.

---

# 14. Order States

Initial business-facing states:

```
PENDING_PAYMENT
CONFIRMED
DISPATCHED
DELIVERED
CANCELLED
```

Internally, payment and fulfilment should be capable of becoming separate state dimensions.

---

# 15. Product Image Requirements

System may:

- improve lighting
- crop
- improve clarity
- simplify background

System must not:

- hide defects
- change condition
- add nonexistent product features
- misrepresent used items as new

Original image must remain available.

---

# 16. Security Requirements

At minimum:

- authentication
- business-level authorization
- resource isolation
- webhook verification
- secrets protection
- event deduplication
- idempotency
- audit logs

AI must never act as authorization boundary.

---

# 17. External Integration Requirements

## Meta / WhatsApp

System should be designed for:

- connection
- inbound messages
- outbound messages
- delivery status
- external account state
- retries
- reconciliation

External success and internal failure must be recoverable.

---

# 18. Non-Functional Requirements

## Reliability

Message processing must tolerate:

- duplicate webhooks
- retries
- temporary external outages

## Scalability

Architecture should allow future:

- businesses in other states/countries
- additional currencies
- different payment providers
- different messaging channels

## Observability

Team should be able to trace:

```
incoming webhook
→ conversation
→ agent decision
→ tool call
→ outgoing message
```

## Auditability

Important state changes must identify:

- actor
- action
- resource
- timestamp

---

# 19. Success Metrics

We should measure product value, not only technical activity.

## Activation

- merchant signup completion
- WhatsApp connection success
- time to first product
- time to first usable conversation

## Usage

- active merchants
- customer conversations
- AI-handled conversations
- human takeovers
- products added

## Sales

- serious enquiries
- orders created
- conversation-to-order conversion
- recovered sales after follow-up
- recovered revenue

## Retention

- repeat customer activity
- returning merchant usage
- merchant weekly activity

## Efficiency

- AI handle rate
- average merchant interventions
- response time
- estimated merchant time saved

---

# 20. Pilot Plan

Initial pilot:

> approximately 3–4 merchants
> 

Pilot merchants should represent different commerce behaviours where possible.

Examples:

- physical appliance merchant
- active WhatsApp seller
- merchant already using online marketplaces
- merchant with referral-heavy business

The objective is not maximum users.

The objective is maximum learning.

---

# 21. Pilot Questions

During pilot we need to discover:

- Do merchants trust AI talking to customers?
- Which conversations require merchant intervention?
- Does follow-up actually recover sales?
- Which follow-up timing works?
- How often is product information inaccurate?
- Do merchants maintain product availability?
- Is product photo enhancement useful?
- Do merchants use generated Status content?
- Does natural-language product entry work?
- Can conversations reliably become orders?
- Do merchants understand daily summaries?
- Which actions should remain approval-based?

---

# 22. MVP Exit Criteria

Commerce MVP should not progress into major payment development solely because features are completed.

We should see evidence that:

- merchants repeatedly use the platform
- merchant workload is reduced
- AI accuracy is acceptable
- merchants allow the system to handle real customers
- orders are created from real conversations
- follow-ups produce measurable responses
- merchants understand the value
- at least some merchants indicate willingness to pay

---

# 23. Major Risks

## Risk — Merchant does not trust AI

Mitigation:

- human takeover
- gradual automation
- transparent controls

## Risk — Product information becomes stale

Mitigation:

- simple availability updates
- merchant confirmations
- uncertainty escalation

## Risk — WhatsApp integration complexity

Mitigation:

- explicit external/internal state
- retries
- reconciliation
- strong onboarding UX

## Risk — AI becomes too autonomous

Mitigation:

- tool boundaries
- policy engine
- deterministic pricing
- escalation

## Risk — Merchant gets overwhelmed

Mitigation:

- WhatsApp-first interaction
- strong defaults
- natural language
- secondary dashboard

## Risk — Follow-up feels like spam

Mitigation:

- contextual triggers
- frequency limits
- consent/policy compliance
- merchant controls

---

# 24. Open Questions

The following are not yet fully resolved:

- exact merchant signup experience
- whether merchant assistant starts before or after WhatsApp coexistence setup
- exact Meta eligibility requirements
- exact product availability model
- how physical sales are captured during MVP
- default follow-up timing
- which actions can auto-send without merchant approval
- order confirmation rules
- inventory handling depth
- pricing model for merchant subscription
- WhatsApp usage cost allocation

These should be resolved through individual feature PRDs and pilot evidence.

---

# 25. Product Decision Rule

If a feature does not materially help us validate:

> merchant time savings
> 
> 
> sales conversion
> 
> customer retention
> 
> reliable commerce automation
> 

it should not block the Commerce MVP.

---

# 26. Definition of MVP

The Commerce MVP is successful when a real merchant can:

```
Join
↓
Connect their WhatsApp Business
↓
Add products
↓
Receive a real customer enquiry
↓
Let AI assist the conversation
↓
Take over if necessary
↓
Follow up when customer disappears
↓
Turn customer agreement into an order
↓
Understand what happened without maintaining complex software
```

That is the first complete product we are trying to prove.