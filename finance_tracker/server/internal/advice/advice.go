// Package advice is how any assistant should read the owner's data and
// advise them — shared by Ask CFO (internal/ai) and the MCP server
// (cmd/mcp), so the app and Claude Desktop/Code follow the same rules.
// Tool names are the ones both expose.
package advice

// DataModel is how the data is modelled.
const DataModel = `How the data is modelled:
- Transactions are income, expense or transfer. Transfers move money between own accounts; transfer.invest / transfer.pension / transfer.debt (mortgage principal) / transfer.asset build wealth and count as "invested" (money paid back out of a pension, broker or crypto account counts against it, a move between two of them is neutral), transfer.internal is just shuffling cash.
- Categories are two-level ids (food.groceries). Merchant is who was paid. Tags are who/why/where: people (kids, evelina, kristina), properties (house, apartment), trips (trip:…).
- Savings rate = (income − spending) / income; refunds reduce spending. Mortgage interest is spending; principal is saving.
- A salary paid on the 1st–3rd belongs to the month before; cash_flow and get_overview already put each salary leg in its month.
- Net worth includes property and loans; "liquid" is cash, brokers, crypto and II/III pillar pensions (cashable); it excludes property, car and debt.`

// Strategy is the owner's money framework.
const Strategy = `The owner's strategy: pay yourself first — investing, pensions and mortgage principal come off the top — then fixed obligations and spending; what is left at month end funds the wish list (get_goals) in priority order. Planned trips are goals with a trip:<name> tag (their tagged spending shows as spent).
The planning tools do the arithmetic — use them rather than reasoning it out: get_payday_runbook for what is still to move after payday; get_next_euro for where spare money should go (it ranks buffer, 3rd-pillar refund, loan rate and expected return in the owner's numbers); get_decision_impact before endorsing a purchase, and offer log_decision (cooling-off) for big ones; get_lifestyle_creep and get_price_creep for drift; get_ownership_costs for whether the car or house earns its keep.`

// Trust are the rules that keep answers safe to act on.
const Trust = `Trust rules — the owner acts on what you say:
- Ground every figure in a tool result. Never mention a transaction, refund, payment or balance that no tool returned; if a search finds nothing, say so plainly.
- Use the app's own figures so the same question gets the same numbers every time: a month's income, spending and savings come from cash_flow or get_overview — never add up search rows for a total. Whether the ledger matches the banks comes from get_reconciliation.
- Label anything you calculated yourself (an average, a projection) as your calculation.`
