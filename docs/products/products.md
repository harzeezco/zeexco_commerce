# 03 Product

This section explains what the product is, how it should behave, what is included in the MVP, what comes later, and the language the team should use consistently.

Every product, design and engineering decision should be traceable back to this section and the Research section.

---

# Product Overview

## Product Definition

We are building an intelligent commerce operating layer for busy merchants.

The product helps merchants:

- respond to customers
- present products better
- convert more enquiries into sales
- remember customer context
- follow up with interested customers
- create structured orders from conversations
- retain existing customers
- reduce repetitive business work
- eventually manage payments and money movement

The product should work for merchants who operate:

- physically
- online
- through referrals
- through WhatsApp
- through social media
- through a warehouse or shop
- through a mix of offline and digital channels

The merchant should not have to abandon the workflows and customer relationships that already work.

---

## Core Product Thesis

Many merchants already have:

- products
- customers
- referrals
- WhatsApp conversations
- physical demand
- repeat buyers

Their main problem is not simply:

> “How do I get online?”
> 

The stronger problem is:

> “How do I manage the commercial activity I already have without losing customers, wasting time or missing sales?”
> 

Our product should improve what already works instead of forcing merchants into a completely new commerce model.

---

## Primary Product Value

The product should create value in four major areas.

### 1. Save Merchant Time

Reduce repetitive work such as:

- answering the same questions
- retrieving product information
- sending repeated descriptions
- remembering interested customers
- manually following up
- organizing customer conversations
- recording orders
- creating sales content
- checking what happened during the day

The product should feel like additional business capacity.

---

### 2. Help Merchants Sell More

The product should improve conversion, not only automate responses.

Examples:

- better product presentation
- faster replies
- structured negotiation
- remembering customer context
- following up after price enquiries
- recovering abandoned intent
- helping merchants re-engage existing customers
- turning conversations into structured orders

The product should optimize:

> enquiry → conversation → commitment → sale
> 

not simply:

> message → AI reply
> 

---

### 3. Retain Existing Customers

Existing and referred customers are valuable.

The system should remember:

- who contacted the merchant
- what they wanted
- what they bought
- what they asked about
- whether they disappeared
- whether they complained
- when they last interacted
- whether they should be followed up with

The system should gradually become the merchant’s customer memory.

---

### 4. Simplify Money Movement

Payments are not the first MVP, but they are a core future capability.

Eventually the product should connect:

```
Customer
↓
Order
↓
Expected payment
↓
Payment received
↓
Settlement
↓
Fulfilment
```

This creates a complete commercial record.

Long term this may include:

- NGN collections
- virtual accounts
- payment confirmation
- payment reconciliation
- supplier payments
- domestic transfers
- cross-border payments
- FX
- stablecoin-based settlement where appropriate
- merchant payment summaries

---

## Product Surfaces

The product is expected to have two major interaction surfaces.

### Merchant Assistant

The merchant talks to our assistant for business actions and business understanding.

Examples:

> “How much did I sell today?”
> 

> “Who needs follow-up?”
> 

> “Add this product.”
> 

> “Which customers asked for this item?”
> 

> “Who has not paid?”
> 

> “Give me today’s summary.”
> 

Later:

> “Pay this supplier.”
> 

> “Show my collections today.”
> 

> “Send money to my supplier in China.”
> 

The merchant should not need to learn dashboards to perform common business tasks.

---

### Merchant WhatsApp Commerce Layer

Customers communicate through the merchant’s own WhatsApp Business identity.

The product connects to that business account through Meta-supported coexistence infrastructure.

The system can then assist with:

- incoming customer questions
- product discovery
- pricing
- permitted negotiation
- customer memory
- follow-up
- order creation
- merchant takeover

The customer should feel like they are dealing with the merchant’s business, not being moved into another platform.

---

## Product Experience Goal

The product should not feel like:

> “business software I need to operate.”
> 

It should feel like:

> “something helping me run my business.”
> 

Most useful work should happen automatically.

The platform exists to support the merchant, not to become another task the merchant must maintain.

---

# Product Principles

These principles should guide product, design and engineering decisions.

## Principle 1 — Merchant Workflow First

We adapt to the merchant’s existing workflow where possible.

Do not redesign the merchant’s entire business because software architecture prefers a cleaner model.

---

## Principle 2 — WhatsApp Is a Primary Work Surface

For many merchants, WhatsApp already functions as:

- inbox
- customer database
- sales channel
- referral channel
- product-sharing channel
- negotiation channel

We should integrate into that behaviour rather than fight it.

---

## Principle 3 — Do Not Make Merchants Learn Complex Software

A busy merchant should not need training in:

- CRM concepts
- analytics terminology
- complex configuration
- advanced dashboards

The product should translate technical capability into business language.

Instead of:

> Conversion rate decreased by 8%.
> 

Prefer:

> 6 customers asked for a price today. 2 bought, 3 still need follow-up, and 1 declined.
> 

---

## Principle 4 — Automate Repetition, Not Judgment

AI should handle repetitive and low-risk tasks.

Examples:

- product retrieval
- common questions
- follow-up reminders
- product descriptions
- simple pricing within approved rules

Human judgment should remain for:

- unusual negotiation
- complaints
- refunds
- fraud
- payment disputes
- sensitive customer issues
- important commercial exceptions

---

## Principle 5 — Merchant Remains in Control

The merchant must be able to:

- pause AI
- take over a conversation
- override a response
- change pricing rules
- approve sensitive actions
- disable automation

The system assists the merchant. It does not replace their authority.

---

## Principle 6 — AI Is Not the Source of Truth

AI may reason, summarize and recommend.

It must not invent business facts.

Important information must come from structured systems such as:

- products
- customers
- orders
- payments
- merchant rules
- inventory
- business settings

The AI should act through tools.

---

## Principle 7 — Product Data Should Be Created Automatically

Do not ask merchants to manually maintain information when the system can infer or capture it safely.

Examples:

- create customer from conversation
- create order from confirmed purchase
- update customer history after sale
- record follow-up state
- store product interest
- capture payment context

Manual data entry should be minimized.

---

## Principle 8 — Every Feature Must Produce Business Value

A feature should clearly answer one of these questions:

- Does this save merchant time?
- Does this help close more sales?
- Does this help retain customers?
- Does this reduce commercial risk?
- Does this simplify money movement?
- Does this improve business understanding?

If not, it should probably not be in the MVP.

---

## Principle 9 — Physical Commerce Matters

The product must not assume all commerce begins online.

A merchant may receive:

- walk-in customers
- calls
- referrals
- warehouse visits
- WhatsApp enquiries
- social media enquiries

The system should eventually unify these forms of commerce rather than optimize only social commerce.

---

## Principle 10 — Build Locally, Architect Globally

Initial validation may happen with Nigerian merchants.

But architecture should avoid unnecessary assumptions tied to:

- one state
- one city
- one currency
- one payment provider
- one merchant category

Business behaviour should be modeled separately from infrastructure that changes by geography.

---

## Principle 11 — Default to Simplicity

If a merchant needs to configure 20 settings before getting value, we failed.

Strong defaults are preferred.

Complexity can exist internally while the merchant sees something simple.

---

## Principle 12 — Trust Before Automation Expansion

The product should earn the right to automate more.

Start with:

- assistance
- recommendations
- low-risk automation

Then expand automation as:

- merchant trust increases
- data quality improves
- product reliability improves

Payments should especially come after trust has been established.

---

# MVP Scope

## MVP Goal

Prove that we can help busy merchants:

> handle customer enquiries more effectively, convert more conversations into sales, and remember customers without creating more administrative work.
> 

The MVP is not intended to be a complete business-management platform.

---

# MVP Core Capabilities

## 1. Merchant Onboarding

Merchant should be able to:

- create account
- create business
- connect WhatsApp Business
- complete minimum required setup
- reach a usable state quickly

---

## 2. WhatsApp Business Coexistence

Merchant keeps:

- existing WhatsApp Business number
- mobile app
- current customer relationships

Our system gains supported platform access through Meta.

---

## 3. Product Creation

Merchant can create products with minimal effort.

Preferred experience:

```
photo
+
basic price information
+
optional lowest acceptable price
↓
AI structures product
↓
AI improves description
↓
AI optionally improves product presentation
↓
merchant confirms
```

Avoid large administrative forms.

---

## 4. Product Intelligence

The system should know:

- product name
- description
- images
- asking price
- minimum price if negotiable
- availability
- relevant variants
- merchant-defined rules

---

## 5. AI Sales Conversation

The AI should be able to:

- identify customer intent
- retrieve relevant products
- answer common product questions
- give approved prices
- understand product interest
- recommend products
- progress toward order creation

The AI must use tools rather than invent business information.

---

## 6. Negotiation

For merchants who negotiate:

```
asking price
minimum acceptable price
```

should be supported.

The AI may negotiate only inside approved rules.

---

## 7. Human Takeover

Conversation states should support at minimum:

```
AI_HANDLING
HUMAN_HANDLING
WAITING_FOR_MERCHANT
PAUSED
CLOSED
```

When merchant responds manually, AI pauses.

---

## 8. Customer Memory

Customer records should be created automatically.

Initial customer record may include:

- name
- phone
- first interaction
- last interaction
- products asked about
- purchase history
- open follow-up
- customer status

---

## 9. Follow-Up

The system should detect cases such as:

```
customer asks price
↓
price sent
↓
customer stops replying
↓
follow-up due
```

Initial MVP may:

- remind merchant
- suggest follow-up text
- optionally send approved automated follow-up

---

## 10. Order Creation

Once purchase intent is confirmed:

```
conversation
↓
product
↓
quantity
↓
final agreed price
↓
customer confirmation
↓
order created
```

Initial statuses:

```
PENDING_PAYMENT
CONFIRMED
DISPATCHED
DELIVERED
CANCELLED
```

Payment may initially be recorded manually.

---

## 11. Product Sales Content

From a product, generate:

- better product description
- customer-ready text
- WhatsApp Status caption
- share-ready image

The merchant should be able to immediately reuse the product for selling.

---

## 12. Daily Merchant Summary

The merchant should receive a business-language summary.

Example:

> Today you had 9 customer enquiries.
> 
> 
> 3 customers bought.
> 
> 2 are waiting for payment.
> 
> 3 should be followed up with.
> 
> 1 conversation needs your attention.
> 

Avoid complex charts as the default interface.

---

# Not in MVP

Do not include initially:

- full POS
- complex logistics marketplace
- full accounting
- advanced ERP
- complex warehouse system
- Instagram integration
- ecommerce storefront
- ticketing
- subscriptions
- complex analytics
- advanced staff management
- broad payment infrastructure
- cross-border payments
- stablecoin settlement
- advanced loyalty programs
- large CRM feature set

These may come later after MVP validation.

---

# MVP Success Criteria

The MVP should answer:

1. Do merchants actually connect and use the system?
2. Does the product reduce response/follow-up workload?
3. Does AI help without creating more merchant supervision?
4. Do merchants trust the system with customer conversations?
5. Does follow-up improve conversion?
6. Does product presentation improve customer engagement?
7. Can the system reliably create structured orders from conversations?
8. Do merchants continue using it after initial testing?
9. Would merchants eventually pay for it?

Initial validation can begin with approximately 3–4 merchants.

---

# Roadmap

The roadmap is hypothesis-driven.

Dates should not be treated as fixed until learning supports them.

---

## Phase 0 — Research and Foundation

### Goal

Understand commerce before building.

### Work

- merchant interviews
- customer research
- Xara research
- Sowo research
- competitor knowledge base
- product principles
- domain modeling
- WhatsApp coexistence research
- Meta API research
- product architecture
- security model

### Exit criteria

We can clearly explain:

- target user
- core problem
- MVP
- first flows
- why the product should exist

---

## Phase 1 — Commerce MVP

### Goal

Help merchants sell and follow up better through WhatsApp.

### Features

- merchant onboarding
- WhatsApp coexistence
- product creation
- product records
- AI sales conversations
- customer memory
- negotiation rules
- human takeover
- follow-up
- order creation
- basic daily summary
- product content generation

### Pilot

3–4 merchants.

### Primary question

Does this create measurable merchant value?

---

## Phase 2 — Commerce Intelligence

### Goal

Move from basic automation to meaningful sales assistance.

Potential features:

- abandoned-intent detection
- conversion recommendations
- customer segmentation
- repeat-purchase opportunities
- intelligent follow-up timing
- better sales copy
- product recommendations
- sales-stage detection
- merchant action recommendations
- stronger business summaries
- smarter customer retention

---

## Phase 3 — Domestic Payments

### Goal

Connect commerce to money.

Potential capabilities:

- merchant NGN collection account
- virtual accounts
- payment requests
- payment confirmation
- order-to-payment reconciliation
- payment history
- merchant balance
- withdrawals
- business payment summaries

### Strategic reason

By this stage:

- merchant trust should exist
- product should already be part of daily workflow
- customers and orders already exist in the system

Payments become an extension of existing commerce rather than a separate product.

---

## Phase 4 — Merchant Financial Operations

Potential capabilities:

- supplier payments
- expenses
- settlement
- reconciliation
- business cash visibility
- merchant financial summaries
- payment automation

---

## Phase 5 — Cross-Border Commerce Payments

Potential capabilities:

- cross-border supplier payments
- FX
- stablecoin settlement where appropriate
- multi-currency balances
- payment routing
- international collections
- settlement providers

This phase must follow applicable regulatory and licensing requirements.

---

## Phase 6 — Geographic Expansion

Expansion sequence:

```
merchant segment
↓
local market
↓
city
↓
state
↓
country
```

Expansion should happen only when the core workflow is repeatable.

---

# Terminology

This section defines the words the product team should use consistently.

## Merchant

The person or business selling goods through:

- physical location
- WhatsApp
- social platforms
- referrals
- other channels

---

## Customer

A buyer or prospective buyer interacting with the merchant.

---

## Business

A merchant commercial entity inside our platform.

A user may eventually belong to more than one business.

---

## Business Membership

Relationship between a user and a business.

Example fields:

```
user
business
role
status
permissions
```

Possible roles:

- owner
- manager
- staff

---

## Merchant Assistant

The assistant the merchant talks to for business actions and business understanding.

This is different from the AI handling customer conversations.

---

## Commerce Agent

The AI system that helps handle customer sales conversations.

It uses structured tools and merchant rules.

---

## WhatsApp Coexistence

Meta-supported setup where the merchant continues using the WhatsApp Business mobile app while our platform also gains supported access to business messaging infrastructure.

---

## Conversation

A customer interaction thread connected to the merchant.

---

## Conversation State

The current responsibility/status of the conversation.

Examples:

```
AI_HANDLING
HUMAN_HANDLING
WAITING_FOR_MERCHANT
PAUSED
ESCALATED
CLOSED
```

---

## Human Takeover

When the merchant or staff assumes control of a conversation and the AI stops replying.

---

## Product

A good the merchant sells.

---

## Product Variant

A version of a product based on characteristics such as:

- size
- colour
- capacity
- model

---

## Asking Price

The merchant’s normal starting price.

---

## Minimum Price

The lowest price the merchant authorizes the system to accept or negotiate toward.

This is a rule, not an AI decision.

---

## Pricing Policy

The deterministic rules controlling how a product may be priced or negotiated.

---

## Customer Memory

Structured customer information built from interactions and transactions.

It may include:

- interests
- purchases
- conversations
- follow-ups
- complaints
- last interaction

---

## Follow-Up

A future action triggered because a customer conversation or order requires additional attention.

---

## Abandoned Intent

A customer showed meaningful purchase interest but stopped progressing.

Example:

```
asked product
↓
asked price
↓
received price
↓
no response
```

---

## Order

A structured commercial commitment between merchant and customer.

An order should contain:

- customer
- product
- quantity
- agreed price
- status

---

## Order State

Initial states:

```
PENDING_PAYMENT
CONFIRMED
DISPATCHED
DELIVERED
CANCELLED
```

---

## Lead

A potential customer who has shown meaningful commercial interest.

Use carefully.

Do not treat every message as a lead.

---

## Conversion

A meaningful commercial progression.

Final sales conversion usually means:

> enquiry/customer interaction → completed purchase
> 

---

## Referral Customer

A customer introduced to the merchant through an existing relationship or another customer.

---

## Returning Customer

A customer who has previously completed a purchase and returns.

---

## At-Risk Customer

A customer whose behaviour suggests possible loss of future business.

This should eventually be based on actual merchant-specific data rather than arbitrary rules.

---

## Business Summary

Natural-language explanation of important business activity.

Example:

> “You made 4 sales today worth ₦185,000. Two customers still need payment follow-up.”
> 

This is preferred over technical analytics as the default merchant experience.

---

## Tool

A deterministic function the AI can call.

Examples:

```
search_products
get_customer
create_order
schedule_followup
```

The AI reasons; tools perform controlled actions.

---

## Automation

System behaviour triggered by deterministic conditions.

Example:

```
customer inactive after price sent
↓
follow-up rule triggers
```

Automation is not the same thing as AI reasoning.

---

## Escalation

A conversation or action transferred from AI to a human because policy or uncertainty requires merchant judgment.

---

## Source of Truth

The authoritative structured system for a fact.

Examples:

- product database for product price
- order database for order state
- payment provider for payment status

The LLM is never the source of truth.

---

## Merchant Rule

A merchant-defined business constraint.

Examples:

- lowest price
- delivery area
- business hours
- refund policy
- products available
- actions requiring approval

---

## Daily Summary

A concise natural-language recap of:

- sales
- enquiries
- follow-ups
- payments
- customer actions requiring attention

---

## Payment

A financial transaction linked to a customer/order or business action.

Payments belong to a later phase of the product.

---

## Virtual Account

A bank account identifier assigned for collections and reconciliation.

Planned for the payment phase.

---

## Settlement

Movement of collected funds into the merchant’s usable balance or destination account.

---

## Cross-Border Payment

Movement of value between different countries/currencies.

This is a future phase, not part of the initial commerce MVP.

---

# Product Rule

Whenever a team member proposes a feature, ask:

> What merchant problem from our research does this solve?
> 

Then:

> Does it save time, increase sales, retain customers, reduce risk, or simplify money movement?
> 

If we cannot answer those questions clearly, the feature should not enter the product backlog yet.