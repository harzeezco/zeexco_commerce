# 04 User Flows

This section describes how users move through the product.

These flows are not screen designs.

They define:

- what the user is trying to achieve
- what the system must do
- what state changes occur
- when AI is allowed to act
- when human intervention is required
- what happens when something fails
- what engineering must preserve

Each flow should eventually map to:

> product requirements → designs → APIs → events → tests → implementation
> 

---

# F001 — Merchant Activation

## Goal

Get a new merchant from:

> no account
> 

to:

> connected WhatsApp Business + active business workspace + first usable product
> 

with as little software learning as possible.

## Primary Actor

Merchant / business owner.

## Trigger

Merchant:

- receives invitation link
- scans onboarding QR
- clicks website CTA
- is onboarded manually by our team

## Entry State

Merchant has:

- no account
- no business workspace
- no WhatsApp connection

## Success State

Merchant has:

- user profile
- business created
- owner membership
- WhatsApp Business connected
- active coexistence connection
- first product added or onboarding ready for product creation
- sales assistant ready

---

## Product Principle

Merchant should not feel like they are signing up for complicated software.

Preferred experience:

> link → WhatsApp → simple setup → connect business → ready
> 

---

## Happy Path

```
Merchant opens onboarding link
↓
Merchant enters/continues through WhatsApp
↓
System identifies WhatsApp number
↓
Create user profile
↓
Create business
↓
Create OWNER membership
↓
Merchant chooses "Connect WhatsApp Business"
↓
Meta Embedded Signup opens
↓
Merchant authorizes business
↓
Merchant selects existing WhatsApp Business account
↓
Coexistence configured
↓
Meta confirms connection
↓
Our backend provisions connection
↓
Chat/customer sync begins where available
↓
Connection becomes ACTIVE
↓
Merchant prompted to add first product
↓
Merchant activation completed
```

---

## Minimum Information

Initial non-financial onboarding should request only what is necessary.

Possible fields:

- merchant name
- business name
- business category
- operating phone
- optional email
- optional physical location

Do not require financial/KYB information before it is needed for payment features.

---

## Connection States

```
NOT_STARTED
CONNECTING
META_AUTHORIZED
PROVISIONING
SYNCING
ACTIVE
FAILED_RECOVERABLE
DISCONNECTED
```

### Important Rule

Meta authorization state and our internal provisioning state must be stored separately.

Example:

```
meta_status = CONNECTED
internal_status = SYNC_FAILED
```

This must not send the merchant back through Meta authorization unnecessarily.

---

## Failure Scenarios

### Merchant abandons signup

System:

- saves progress
- allows resume later

### Meta login fails

Show:

> "We couldn't connect to Meta. Your business setup is saved."
> 

Allow retry.

### WhatsApp already connected elsewhere

Explain:

> "This WhatsApp number is already connected to another Business Platform provider."
> 

Do not show generic error.

### Meta succeeds, our callback fails

System must recognize:

```
META_AUTHORIZED
SYNC_FAILED
```

Show:

> "Your WhatsApp is connected. We couldn't finish syncing. Retry setup."
> 

### Sync takes time

Show:

> "WhatsApp connected. We're importing your business information."
> 

Never send user back to "Connect WhatsApp."

---

## Data Touched

```
User
Business
BusinessMembership
WhatsAppConnection
ExternalAccount
OnboardingProgress
AuditLog
```

---

## Key Events

```
USER_CREATED
BUSINESS_CREATED
BUSINESS_OWNER_ASSIGNED
WHATSAPP_CONNECT_STARTED
WHATSAPP_META_AUTHORIZED
WHATSAPP_PROVISIONING_STARTED
WHATSAPP_SYNC_STARTED
WHATSAPP_CONNECTION_ACTIVE
WHATSAPP_CONNECTION_FAILED
MERCHANT_ACTIVATED
```

---

## Acceptance Criteria

Merchant can:

- complete setup from mobile
- retain existing WhatsApp Business app
- resume onboarding
- recover from failed provisioning
- understand connection status
- reach active state without learning Meta technical concepts

---

# F002 — Product Creation

## Goal

Allow a busy merchant to turn a raw product into a sales-ready structured product with minimal work.

## Primary Actor

Merchant.

## Trigger

Merchant:

- sends product photo to Merchant Assistant
- uploads through dashboard
- forwards product information
- manually chooses "Add product"

## Success State

Product is:

- structured
- stored
- searchable by agent
- available for customer conversations
- ready for Status/direct sharing

---

## Preferred Experience

```
Merchant sends photo
+
"LG washing machine 280k, last 260k"
↓
System analyzes
↓
Product draft created
↓
AI generates title + description
↓
Image enhanced without changing reality
↓
Pricing rules extracted
↓
Merchant reviews
↓
Merchant confirms
↓
Product saved
↓
Sales-ready content generated
```

---

## Required Product Data

Minimum:

```
name
business_id
asking_price
availability
primary_image
```

Optional/derived:

```
description
minimum_price
negotiable
category
variants
stock_quantity
condition
attributes
```

---

## Pricing Logic

If merchant says:

> "280k, last 260k"
> 

System structures:

```
asking_price = 280000
minimum_price = 260000
negotiation_enabled = true
```

AI must never change `minimum_price`.

---

## Image Rules

Allowed:

- crop
- sharpen
- improve lighting
- remove distracting background
- improve composition
- make presentation cleaner

Not allowed:

- hide defects
- invent packaging
- change product colour
- alter actual product condition
- make used product appear new

---

## Product Creation States

```
DRAFT
PROCESSING
NEEDS_REVIEW
ACTIVE
OUT_OF_STOCK
ARCHIVED
```

---

## Follow-On Actions

After product creation, merchant can receive:

> "Your product is ready."
> 

Actions:

- Send to customer
- Share to WhatsApp Status
- View product
- Add another product

---

## AI Tools

```
analyze_product_image
generate_product_title
generate_product_description
enhance_product_image
create_product_draft
save_product
get_pricing_policy
```

---

## Acceptance Criteria

A merchant should be able to create a basic product with:

> image + one natural-language message
> 

without filling a long form.

---

# F003 — Sales Conversation

## Goal

Help merchants respond to customer enquiries quickly and accurately while reducing merchant work.

## Primary Actors

- Customer
- Commerce Agent
- Merchant

## Trigger

Customer messages merchant's WhatsApp Business number.

## Entry State

WhatsApp connection is ACTIVE.

## Happy Path

```
Customer sends message
↓
Meta webhook received
↓
Customer resolved/created
↓
Conversation resolved/created
↓
Conversation policy checked
↓
AI allowed to respond
↓
Intent classified
↓
Relevant tools called
↓
Response generated from real business data
↓
Message sent through merchant's WhatsApp identity
↓
Conversation state updated
```

---

## Example

Customer:

> "Do you have washing machine?"
> 

Agent:

```
search_products("washing machine")
```

If available:

> "Yes. We currently have these options available..."
> 

If none:

> "There isn't one currently listed. I can ask the merchant if another one is available."
> 

The model must not invent inventory.

---

## Customer Record Creation

If customer does not exist:

```
Customer created automatically
first_seen_at = now
source = WHATSAPP
```

---

## Intent Types

Initial examples:

```
PRODUCT_SEARCH
PRICE_REQUEST
AVAILABILITY
NEGOTIATION
DELIVERY
PURCHASE_INTENT
COMPLAINT
GENERAL_QUESTION
HUMAN_REQUEST
UNKNOWN
```

---

## AI Response Policy

AI may automatically handle:

- product availability
- standard price
- product information
- business hours
- standard delivery information
- permitted negotiation

Escalate:

- uncertainty
- complaints
- refunds
- exceptional discounts
- payment disputes
- unclear product condition
- sensitive issues

---

## Success Metrics

- response time
- AI handle rate
- customer progression
- merchant intervention rate
- conversion to meaningful sales stage

---

# F004 — Negotiation

## Goal

Allow AI to participate in price negotiation without violating merchant-defined commercial limits.

## Trigger

Customer requests:

- lower price
- "last price"
- discount
- bulk pricing

## Preconditions

Product has:

```
asking_price
minimum_price
negotiation_enabled
```

## Happy Path

```
Customer asks for discount
↓
Agent retrieves pricing policy
↓
Agent evaluates request
↓
Request within allowed range?
```

### If Yes

Agent may negotiate within rules.

Example:

```
asking = ₦280,000
minimum = ₦260,000
customer offers = ₦270,000
```

Agent can proceed according to policy.

### If Below Floor

Example:

```
customer offers = ₦230,000
minimum = ₦260,000
```

Agent must not accept.

Possible response:

> "The best available price is ₦260,000."
> 

or escalate depending on merchant policy.

---

## Negotiation States

```
NOT_NEGOTIATING
OFFER_MADE
COUNTER_OFFERED
AWAITING_CUSTOMER
MERCHANT_APPROVAL_REQUIRED
AGREED
DECLINED
```

---

## Merchant Rules

Possible future rules:

```
minimum_price
max_discount_percentage
bulk_discount
approval_threshold
auto_negotiation_enabled
```

---

## Hard Rule

LLM never determines merchant price boundaries.

Pricing rules come from structured merchant policy.

---

# F005 — Human Takeover

## Goal

Allow merchant/staff to take control of any customer conversation without AI interference.

## Trigger

Any of:

- merchant manually replies
- customer asks for human
- AI escalates
- policy requires human approval
- complaint/refund/payment dispute

## Happy Path

```
AI handling conversation
↓
Human message sent
↓
HUMAN_MESSAGE_SENT event
↓
AI paused
↓
conversation.owner = HUMAN
↓
merchant continues conversation
```

---

## Conversation States

```
AI_HANDLING
WAITING_FOR_MERCHANT
HUMAN_HANDLING
PAUSED
ESCALATED
CLOSED
```

---

## Resume Logic

AI should not automatically resume sensitive conversations.

For low-risk conversations:

```
merchant inactive
+
configured timeout reached
+
conversation eligible for AI
→ AI may resume
```

For high-risk conversations:

> human must explicitly return control.
> 

---

## Escalation Severity

```
LOW
MEDIUM
HIGH
CRITICAL
```

Examples:

### Low

Customer asks unusual product question.

### Medium

Customer requests discount below allowed floor.

### High

Customer complains about damaged product.

### Critical

Payment dispute, fraud suspicion, refund conflict.

---

## Merchant Experience

Merchant should receive natural-language alert:

> "A customer needs your decision. They are asking for ₦15,000 below your approved minimum price."
> 

Not:

> "Escalation code POLICY_PRICE_402."
> 

---

# F006 — Follow-Up

## Goal

Prevent interested customers from being forgotten and recover sales that would otherwise be lost.

## Trigger Examples

```
price sent + no reply
product shown + no reply
invoice/payment request unpaid
customer said "I'll get back to you"
requested product becomes available
previous customer becomes inactive
```

---

## Important Distinction

Follow-up should not mean:

> spam everyone.
> 

The system must understand why follow-up is appropriate.

---

## Example Flow

```
Customer asks about washing machine
↓
Price sent
↓
Customer does not reply
↓
Follow-up timer starts
↓
System evaluates context
↓
Follow-up recommended
↓
Merchant approval OR approved automation
↓
Message sent
↓
Result tracked
```

---

## Follow-Up Reasons

```
PRICE_NO_RESPONSE
PRODUCT_NO_RESPONSE
PAYMENT_PENDING
PRODUCT_BACK_IN_STOCK
RETURNING_CUSTOMER
INACTIVE_CUSTOMER
ORDER_CHECK
POST_DELIVERY
```

---

## Follow-Up Intelligence

System should consider:

- product still available?
- customer already purchased?
- customer complained?
- previous follow-up already sent?
- how long since last message?
- customer blocked/opted out?
- merchant-defined limits?
- messaging cost?
- WhatsApp rules?

---

## Merchant Summary Example

> "4 customers need follow-up today. Three asked for prices yesterday and one said they would return this week."
> 

Action:

> Follow all
> 
> 
> Review
> 
> Ignore
> 

---

## Metrics

Track:

```
followups_sent
customer_replied
sale_recovered
followup_conversion
followup_revenue
```

This is important for proving merchant value.

---

# F007 — Order Creation

## Goal

Convert confirmed purchase intent into a structured order without requiring merchant data entry.

## Trigger

Customer clearly agrees to buy.

Examples:

> "I'll take it."
> 

> "Send me account details."
> 

> "I want two."
> 

> "Okay, deliver tomorrow."
> 

## Happy Path

```
Customer confirms intent
↓
Agent identifies:
  customer
  product
  quantity
  final agreed price
↓
Missing information?
```

If missing:

> ask only for missing information.
> 

Example:

> "What delivery location should we use?"
> 

Then:

```
order draft created
↓
customer/merchant confirmation if required
↓
order created
↓
status = PENDING_PAYMENT
```

---

## Order Data

```
Order
- id
- business_id
- customer_id
- conversation_id
- status
- subtotal
- discount
- total
- payment_status
- fulfilment_status
- created_at
```

```
OrderItem
- order_id
- product_id
- quantity
- unit_price
- agreed_price
```

---

## Initial Order States

```
PENDING_PAYMENT
CONFIRMED
DISPATCHED
DELIVERED
CANCELLED
```

Internally, payment and fulfilment state should eventually be separated.

Example:

```
order_status
payment_status
fulfilment_status
```

Rather than one overloaded field.

---

## MVP Payment Handling

Payment infrastructure is not required for first commerce MVP.

Initial options:

- merchant manually marks payment received
- order can remain pending payment
- system records payment status without moving money

Future:

```
Order
↓
Payment Request
↓
Virtual Account / Payment Rail
↓
Payment Confirmed
↓
Automatic reconciliation
```

---

## Post-Order Actions

Once order is created:

- update customer history
- record product interest/purchase
- update stock where relevant
- schedule fulfilment reminders
- schedule post-delivery follow-up
- update merchant summary

---

# End-to-End MVP Lifecycle

These seven flows connect into one commerce lifecycle:

```
F001 Merchant Activation
        ↓
F002 Product Creation
        ↓
F003 Sales Conversation
        ↓
F004 Negotiation
        ↓
F005 Human Takeover
        ↕
F006 Follow-Up
        ↓
F007 Order Creation
        ↓
Future: Payment
        ↓
Fulfilment
        ↓
Retention / Repeat Purchase
```

The important product rule is:

> These are not seven disconnected features.
> 

They are seven stages of one merchant commerce system.

---

# Cross-Flow Product Rules

## 1. Context Must Persist

Customer information should survive across:

- conversations
- follow-ups
- orders
- future purchases

## 2. Merchant Rules Override AI

Pricing, permissions, stock and approvals remain deterministic.

## 3. Never Ask Twice for Known Information

If the system already knows:

- customer number
- product
- agreed price

do not ask the merchant to re-enter them.

## 4. Every Flow Must Be Recoverable

If external integrations fail, preserve successful progress.

## 5. Every Important State Change Generates an Event

Examples:

```
CUSTOMER_CREATED
MESSAGE_RECEIVED
PRODUCT_INTEREST_DETECTED
PRICE_SENT
FOLLOWUP_DUE
HUMAN_TAKEOVER
ORDER_CREATED
ORDER_CONFIRMED
```

## 6. Every AI Action Must Be Auditable

We should be able to answer:

> What did the AI do?
> 
> 
> Why?
> 
> Which business data did it use?
> 
> Was the merchant involved?
> 
> What changed?
> 

## 7. Merchant-Facing Language Must Remain Simple

Internally:

> `FOLLOWUP_DUE`
> 

Merchant sees:

> "This customer asked for a price yesterday and hasn't replied."
> 

The product should translate technical state into business meaning.

---

# What Comes After These Flows

Once these flows are validated, the next user flows should likely be:

```
F008 Delivery / Fulfilment
F009 Daily Business Summary
F010 Customer Retention / Repeat Sale
F011 Merchant Payment Activation
F012 Customer Payment
F013 Payment Reconciliation
F014 Domestic Supplier Payment
F015 Cross-Border Supplier Payment
```

Those should not be added to MVP until the first commerce lifecycle is proven.
# WhatsApp browser registration

The WhatsApp CTA opens `${PUBLIC_FRONTEND_BASE_URL}/register?token=<opaque>`.
The frontend can validate the token with
`GET /api/v1/whatsapp/onboarding/validate?token=<opaque>` and submits the same
required token to `POST /api/v1/whatsapp/onboarding/register`. First name, last
name, and business name are required; email is optional and password is not
accepted. Validation is repeated during registration. Only the server-side
onboarding session supplies the WhatsApp identity. User creation, OWNER
membership, identity linking, token consumption, and audits commit in one
PostgreSQL transaction. Web registration remains independently available at
`POST /api/v1/auth/register` with required email and password credentials.

# WhatsApp-native merchant registration

The browser flow remains the production fallback. When
`WHATSAPP_MERCHANT_REGISTRATION_FLOW_ID` is configured, the existing onboarding
delivery path launches `docs/flows/merchant-registration.flow.json` inside
WhatsApp:

```text
Unknown Meta-verified merchant
        -> create/reuse REGISTRATION_PENDING onboarding session
        -> generate opaque single-use onboarding token
        -> interactive Flow CTA (token is non-visible flow_token)
        -> REGISTRATION screen inside WhatsApp
        -> encrypted and signed data_exchange POST /api/v1/whatsapp/flows
        -> Flow RSA/AES-GCM protocol adapter
        -> existing onboarding Complete service
        -> User + Business + OWNER membership + identity link + audit events
        -> FIRST_PRODUCT_PENDING
```

The outbound Cloud API payload uses `flow_message_version: "3"`, the configured
Flow ID, `flow_action: "navigate"`, entry screen `REGISTRATION`, and the validated
`WHATSAPP_FLOW_MODE` (`published` by default, or `draft` for testing) as the
interactive action parameter `mode`. Meta carries
the exact server-generated token as `flow_token`; it is not a form component.
The form submits only first name, last name, business name, and optional email.
It cannot submit a WhatsApp ID, phone number, user ID, business ID, or password.

The endpoint hashes `flow_token`, locks its onboarding row, and derives the
trusted WhatsApp identity from that row. The existing transaction and lifecycle
transition remain the idempotency boundary. If Flow delivery fails, Zeexco uses
the existing browser CTA with the same token. The browser validation and
registration endpoints remain available independently.

