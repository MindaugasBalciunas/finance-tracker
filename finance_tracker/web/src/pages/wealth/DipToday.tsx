import { createContext, useContext } from 'react'
import clsx from 'clsx'
import { Link } from 'react-router-dom'
import { eur, shortDate } from '../../lib/format'
import { amount, PLAN_WORDS, closeness, dueRungs, rungColor, orderCost, orderLine, ordersByFund, px, standingOrders } from '../../lib/dips'
import type { DipFund, DipLevel, DipPlan, DipPlanName } from '../../lib/types'
import { Card, Meter, useToast } from '../../components/ui'

// ── Dip plan: what to do now ────────────────────────────────────────
//
// The answer to "do I buy, and how much?" in one card, before any detail:
// money due now (a rung reached and its third not bought), else nothing
// extra plus the limit orders to keep at the broker, and the reserve left.
// Buying happens at the broker: once the trade is in the app (IBKR sync, or
// added by hand) the plan sees it and stops asking for that third.

/** Which plan the page shows: the ETFs, the single-stock bets, or bitcoin. */
export const PlanCtx = createContext<DipPlanName>('funds')

/** The emergency fund card on Independence, scrolled to and lit up. */
export const SAFETY_NET = '/wealth/fi?focus=emergency'

export function DipToday({ plan, onSetup }: { plan: DipPlan; onSetup: () => void }) {
  const funds = plan.funds.filter((f) => !f.error)
  const due = funds.filter((f) => f.todo.kind === 'buy')
  const stopped = funds.filter((f) => f.todo.kind === 'stop')
  const orders = standingOrders(funds)
  const noReserve = plan.reserve <= 0
  // The reserve is cash above the safety net: never buy into the emergency fund.
  const capped = plan.has_net && plan.afford < plan.buy_now
  const after = PLAN_WORDS[useContext(PlanCtx)].after
  const tone = due.length > 0 ? (capped && plan.afford <= 0 ? 'warn' : 'good') : stopped.length > 0 ? 'bad' : undefined

  return (
    <Card title="What to do now" icon="target" color={tone ? `var(--hue-${tone})` : undefined}>
      <div className={clsx('text-2xl font-bold tracking-tight', due.length === 0 ? 'text-ink' : tone === 'warn' ? 'text-warn' : 'text-good')}>
        {due.length === 0 ? 'Nothing extra to buy today' : !capped ? `Buy ${eur(plan.buy_now)} now` : plan.afford > 0 ? `Buy up to ${eur(plan.afford)} now` : 'No free cash to buy with'}
      </div>
      <p className="mt-1 text-sm text-ink2">
        {due.length > 0
          ? capped
            ? plan.afford > 0
              ? `Your plan says ${eur(plan.buy_now)}, but only ${eur(plan.afford)} is free above your safety net. Buy up to that at your broker; once the trade is in the app the plan counts it.`
              : `Your plan says ${eur(plan.buy_now)} is due, but no cash is free above your safety net${after ? ` after ${after}` : ''}. Nothing to buy until there is.`
            : 'A rung of your plan is reached. Buy at your broker at today’s price, from cash above your safety net; once the trade is in the app the plan counts it.'
          : noReserve
            ? 'Set the cash you keep aside for falls to see amounts and orders.'
            : funds.length === 0
              ? 'Prices couldn’t be loaded right now. Try again in a few minutes.'
              : orders.length === 0
                ? 'Every third of the reserve is in for this fall. Keep the monthly amount going.'
                : `Keep ${orders.length === 1 ? 'the limit order' : `these ${orders.length} limit orders`} at your broker; they buy by themselves if prices fall.`}
        {plan.monthly > 0 && <> Every month, whatever happens: <b className="tnum text-ink">{eur(plan.monthly)}</b> {plan.monthly_from === 'transfers' ? 'as you usually move to your brokers' : 'as set in Investments'}.</>}
      </p>

      {due.length > 0 && (
        <ul className="mt-3 space-y-2">
          {due.map((f) => <DueFund key={f.symbol} f={f} />)}
        </ul>
      )}

      {stopped.map((f) => (
        <p key={f.symbol} className="mt-3 rounded-xl border border-bad/30 bg-bad/5 px-3 py-2 text-sm text-bad">{f.todo.text}</p>
      ))}

      {noReserve ? (
        <button className="btn-primary mt-3 h-10 px-4 text-sm" onClick={onSetup}>Set your reserve</button>
      ) : (
        <div className="mt-4">
          <div className="flex items-baseline justify-between text-xs text-muted">
            <span>Reserve for falls</span>
            <span className="tnum"><b className="text-ink">{eur(plan.left)}</b> of {eur(plan.reserve)} left</span>
          </div>
          {plan.has_net && <SafetyNet plan={plan} />}
          {plan.missing > 0 && <p className="mt-1 text-xs text-muted">{plan.missing === 1 ? 'One fund is' : `${plan.missing} funds are`} left out: prices didn’t load.</p>}
          <Meter value={plan.left} max={plan.reserve} goal className="mt-1" />
        </div>
      )}

      {orders.length > 0 && <Orders orders={orders} />}
    </Card>
  )
}

/** Where the reserve comes from: cash above the emergency fund's target. */
function SafetyNet({ plan }: { plan: DipPlan }) {
  const words = PLAN_WORDS[useContext(PlanCtx)]
  const spare = Math.max(0, plan.spare) // a later plan's share can be used up by the earlier plans' reserves
  return (
    <div className="mt-2 space-y-1 text-xs">
      <div className="flex items-baseline justify-between text-muted">
        <span>Cash above your <Link to={SAFETY_NET} className="text-accent underline-offset-2 hover:underline">safety net (emergency fund {eur(plan.safety_net)})</Link></span>
        <span className={clsx('tnum font-semibold', plan.short > 0 ? 'text-warn' : 'text-good')}>{eur(spare)}</span>
      </div>
      {words.after && <p className="text-muted">{plan.spare <= 0 ? `${words.after[0].toUpperCase()}${words.after.slice(1)} take all of it: this plan comes after them.` : `After ${words.after}: this plan comes after them.`}</p>}
      {plan.short > 0
        ? <p className="text-warn">The reserve left is {eur(plan.short)} more than that cash. Grow the savings above the safety net, or lower the reserve under “{words.edit}”; the emergency fund is never for buying.</p>
        : <p className="text-muted">Covers the reserve left: falls are bought from savings, the emergency fund stays whole.</p>}
    </div>
  )
}

/** One fund with money due: amount, shares at today's price, the rungs. */
function DueFund({ f }: { f: DipFund }) {
  return (
    <li className="rounded-xl bg-good/8 px-3 py-2.5">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="font-semibold text-ink">{f.name || f.symbol} <span className="font-normal text-muted">· {f.symbol}</span></div>
          <div className="text-xs text-ink2">Price now {px(f.price, f.currency)} · ≈ <b className="tnum">{f.todo.shares}</b> shares</div>
        </div>
        <div className="tnum text-lg font-bold text-good">{eur(f.todo.amount ?? 0)}</div>
      </div>
      <ul className="mt-1.5 space-y-0.5 text-xs text-ink2">
        {dueRungs(f).map((l) => (
          <li key={l.key}><span className="mr-1.5 inline-block h-2 w-2 rounded-full" style={{ background: rungColor(l) }} aria-hidden />{eur(l.tranche)} for the {l.label} rung ({px(l.price, f.currency)})</li>
        ))}
      </ul>
    </li>
  )
}

/** The colour key shared by the order pills, the ladder and the chart. */
export function ToneKey() {
  return (
    <p className="mt-2 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted">
      <span className="inline-flex items-center gap-1"><span className="h-2.5 w-2.5 rounded-sm bg-good" /> buy now</span>
      <span className="inline-flex items-center gap-1"><span className="h-2.5 w-2.5 rounded-sm bg-warn" /> within 5%: next to fill</span>
      <span className="inline-flex items-center gap-1"><span className="h-2.5 w-2.5 rounded-sm bg-sunken ring-1 ring-line" /> further down</span>
    </p>
  )
}

const PILL = {
  buy: 'bg-good/15 text-good ring-1 ring-good/40',
  near: 'bg-warn/15 text-ink ring-1 ring-warn/50',
  far: 'bg-sunken text-ink',
} as const

/** The standing limit orders, one line per fund with all its prices,
 *  ready to type into the broker. */
function Orders({ orders }: { orders: { fund: DipFund; level: DipLevel }[] }) {
  const toast = useToast()
  const groups = ordersByFund(orders)
  const copy = () => {
    const text = orders.map((o) => orderLine(o.fund, o.level)).join('\n')
    // The clipboard needs a secure page (https or localhost); on plain-http
    // LAN access it is missing, so say so rather than doing nothing.
    if (!navigator.clipboard) return toast('Copy needs the app over https', 'bad')
    navigator.clipboard.writeText(text).then(() => toast('Orders copied'), () => toast('Copy failed', 'bad'))
  }
  return (
    <div className="mt-4">
      <div className="flex items-center justify-between gap-2">
        <div className="text-xs font-semibold uppercase tracking-wide text-muted">Limit orders to keep</div>
        <button className="btn-ghost h-10 px-3 text-sm" onClick={copy}>Copy all</button>
      </div>
      <ul className="divide-y divide-line text-sm">
        {groups.map(({ fund: f, levels }) => (
          <li key={f.symbol} className="py-2.5">
            <div className="flex items-baseline justify-between gap-3">
              <span className="font-semibold text-ink">{f.symbol.split('.')[0]}</span>
              <span className="tnum text-xs text-muted">{levels.length === 1 ? '1 order' : `${levels.length} orders`} · {amount(levels.reduce((a, l) => a + orderCost(l), 0), f.currency)}</span>
            </div>
            <div className="mt-1 flex flex-wrap gap-1.5">
              {levels.map((l) => (
                <span key={l.key} className={clsx('tnum whitespace-nowrap rounded-lg px-2 py-1 text-xs', PILL[closeness(l)])}
                  title={`${l.label} · ${eur(l.tranche)} third${l.exact && l.exact !== l.shares ? ` · ${l.exact} with fractions` : ''}`}>
                  <span className="mr-1 inline-block h-2 w-2 rounded-full align-[0px]" style={{ background: rungColor(l) }} aria-hidden />
                  {l.shares > 0 ? <b>{l.shares} ×</b> : <span className="text-muted">&lt;1 ×</span>} {px(l.price, f.currency)}
                  {l.shares > 0 && <> = <b>{amount(orderCost(l), f.currency)}</b></>}
                  <span className="text-muted"> · {Math.round(l.away * 100)}%</span>
                </span>
              ))}
            </div>
          </li>
        ))}
      </ul>
      <ToneKey />
      <p className="mt-1 text-xs text-muted">Shares × limit price = order total · how far below today. From the close of {shortDate(orders[0].fund.as_of)}.</p>
    </div>
  )
}
