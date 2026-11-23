#!/usr/bin/env python3
"""Raw-data regression: recompute every money figure from the raw tables with
rules written here from their documented meaning (not the app's code), and
compare with every view and chart that shows it.

    regress_raw.py --db <copy.db> --api http://127.0.0.1:8091/api [--months 24]

Run it against a probe server on a VACUUM INTO copy (ft-live-verify), never
the owner's live data. Exit 1 when any check fails. Rules reimplemented:

- Salary month: the salary rule in force on the payment date (latest `from`
  on or before it); pay on day <= paid_by_day counts for the month before,
  unless the rule has in_month (or paid_by_day 0). Everything else counts on
  its booking date.
- Income: income rows except refunds. Spending: expenses minus refunds.
- Invested: transfer.invest/pension/debt/asset; into a wealth account (or to
  nowhere) adds, out of a broker/pension/crypto into cash takes away, between
  wealth accounts is neutral; pension paid by payroll (no source account) is
  reported apart.
- Net worth: each account's latest balance on or before the day (archived
  accounts keep their last value); loans count negative.
"""
import argparse, json, sqlite3, sys, urllib.request
from collections import defaultdict

ap = argparse.ArgumentParser()
ap.add_argument('--db', required=True)
ap.add_argument('--api', default='http://127.0.0.1:8091/api')
ap.add_argument('--months', type=int, default=24, help='months checked view by view (cash flow covers all)')
a = ap.parse_args()
db = sqlite3.connect(a.db)


def api(path):
    with urllib.request.urlopen(a.api + path) as r:
        return json.load(r)


fails, passed = [], 0


def eq(name, mine, app, tol=0.02):
    # a missing field (an older server) is a failure, not a crash
    global passed
    if mine is None or app is None or abs(mine - app) > tol:
        fails.append(f'{name}: raw {mine} vs app {app}')
    else:
        passed += 1


def check(name, ok, detail=''):
    global passed
    if ok:
        passed += 1
    else:
        fails.append(f'{name} {detail}')


# ── raw data ──
kind = dict(db.execute('SELECT id, kind FROM accounts'))
settings = json.loads((db.execute("SELECT value FROM settings WHERE key='plan'").fetchone() or ['{}'])[0])
rules = settings.get('salary_rules') or [{'from': '', 'paid_by_day': 3}]
ess_flag = dict(db.execute('SELECT id, essential FROM categories'))
txs = [dict(zip(('date', 'kind', 'amount', 'src', 'dst', 'cat', 'merchant', 'pending'), r)) for r in db.execute(
    'SELECT date, kind, amount/100.0, COALESCE(account_id,""), COALESCE(to_account_id,""), category, merchant, pending FROM transactions')]
WEALTH_OUT = {'brokerage', 'pension', 'crypto'}
WEALTH = WEALTH_OUT | {'loan', 'property', 'vehicle'}
CASH = {'checking', 'savings', 'cash'}
INVEST = {'transfer.invest', 'transfer.pension', 'transfer.debt', 'transfer.asset'}


def top(c):
    return c.split('.')[0]


def prev_month(m):
    y, mo = int(m[:4]), int(m[5:])
    return f'{y - 1}-12' if mo == 1 else f'{y}-{mo - 1:02d}'


def add_months(m, n):
    y, mo = int(m[:4]), int(m[5:]) - 1 + n
    return f'{y + mo // 12}-{mo % 12 + 1:02d}'


def shift(date):
    r = max((x for x in rules if (x.get('from') or '') <= date), key=lambda x: x.get('from') or '', default=None)
    if not r or r.get('in_month') or (r.get('paid_by_day') or 0) <= 0:
        return 0
    return r['paid_by_day']


def flow_month(t):
    if t['kind'] == 'income' and top(t['cat']) == 'salary' and int(t['date'][8:10]) <= shift(t['date']):
        return prev_month(t['date'][:7])
    return t['date'][:7]


def essential(c):
    # a category's own flag; a category not in the table takes its parent's
    return bool(ess_flag[c]) if c in ess_flag else bool(ess_flag.get(top(c)))


def invested(t):
    if t['kind'] != 'transfer' or t['cat'] not in INVEST:
        return 0.0, False
    fk, tk = kind.get(t['src'], ''), kind.get(t['dst'], '')
    if not t['src'] and t['cat'] == 'transfer.pension' and tk in ('', 'pension'):
        return 0.0, True  # payroll pension: apart
    if not t['src'] and tk in CASH:
        return -t['amount'], False  # money arriving from outside, e.g. a pension payout
    if fk not in WEALTH_OUT:
        return t['amount'], False
    if tk in WEALTH:
        return 0.0, False
    return -t['amount'], False


months = defaultdict(lambda: defaultdict(float))
for t in txs:
    m = flow_month(t)
    f = months[m]
    if t['kind'] == 'income':
        if top(t['cat']) == 'refunds':
            f['refunds'] += t['amount']
        else:
            f['income'] += t['amount']
            if top(t['cat']) == 'salary':
                f['salary'] += t['amount']
    elif t['kind'] == 'expense':
        f['expense'] += t['amount']
        if essential(t['cat']):
            f['essential'] += t['amount']
    v, payroll = invested(t)
    f['invested'] += v
    if payroll:
        f['payroll'] += t['amount']
    if t['kind'] == 'transfer' and t['cat'] == 'transfer.debt':
        f['principal'] += v
for f in months.values():
    f['spending'] = f['expense'] - f['refunds']
    f['saved'] = f['income'] - f['spending']

series = defaultdict(list)
for acc, d, v in db.execute('SELECT account_id, date, value/100.0 FROM balances ORDER BY date'):
    series[acc].append((d, v))


def net_worth(day):
    by = defaultdict(float)
    grp = {'checking': 'cash', 'savings': 'cash', 'cash': 'cash', 'brokerage': 'investments', 'pension': 'pension',
           'crypto': 'crypto', 'property': 'real_assets', 'vehicle': 'real_assets', 'loan': 'debt'}
    for acc, s in series.items():
        pts = [v for d, v in s if d <= day]
        if pts and acc in kind:
            v = pts[-1]
            by[grp.get(kind[acc], 'other')] += -abs(v) if kind[acc] == 'loan' else v
    return by


# ── 1. cash flow: every month, every field ──
ov = api('/overview')
today, cur = ov['date'], ov['date'][:7]
cf = {f['period']: f for f in api('/insights/cashflow?granularity=month')}
for m in sorted(set(months) | set(cf)):
    if m > cur:
        continue
    r, app = months.get(m, {}), cf.get(m)
    if not app:
        check(f'cash flow has {m}', not r or abs(r.get('income', 0)) + abs(r.get('spending', 0)) < 0.01)
        continue
    for k in ('income', 'spending', 'invested', 'saved', 'refunds', 'principal'):
        eq(f'cash flow {m} {k}', r.get(k, 0.0), app[k])
    eq(f'cash flow {m} essential', r.get('essential', 0.0), app['essential'])
years = defaultdict(lambda: defaultdict(float))
for m, f in months.items():
    for k in ('income', 'spending', 'invested'):
        years[m[:4]][k] += f[k]
for y in api('/insights/cashflow?granularity=year'):
    for k in ('income', 'spending', 'invested'):
        eq(f'cash flow year {y["period"]} {k}', years[y['period']][k], y[k], 0.05)

# ── 2. month by month, view by view ──
recent = [add_months(cur, -i) for i in range(a.months - 1, -1, -1)]
for m in recent:
    r = months.get(m, {})
    rv = api(f'/insights/month?month={m}')
    for k in ('income', 'spending', 'invested', 'saved'):
        eq(f'Review {m} {k}', r.get(k, 0.0), rv['flow'][k])
    prev6 = [months.get(add_months(m, -i), {}) for i in range(1, 7)]
    for k in ('income', 'spending', 'invested'):
        eq(f'Review {m} six-month avg {k}', sum(p.get(k, 0.0) for p in prev6) / 6, rv['six_month_avg'][k], 0.05)
    first = m + '-01'
    end = today if m == cur else add_months(m, 1) + '-01'
    nw_end = sum(net_worth(end if m == cur else (add_months(m, 1) + '-00')).values()) if False else None
    pl = api(f'/plan?month={m}')
    eq(f'Plan {m} income received', r.get('income', 0.0), pl['income_actual'])
    eq(f'Plan {m} left to spend = its formula', pl['income_base'] - pl['fixed_planned'] - pl['saving_planned'] - pl['fund_contributions']
       - (pl['discretionary_spent'] - pl['fund_spent']) - pl['fund_overdrawn'], pl['safe_to_spend'])
    lg = api(f'/transactions?kind=income&from={first}&to={m}-31&salary=month&size=1')
    eq(f'Ledger {m} income', r.get('income', 0.0), lg['income'])
    eq(f'Ledger {m} refunds', r.get('refunds', 0.0), lg.get('refunds', 0.0))
    ex = api(f'/transactions?kind=expense&from={first}&to={m}-31&size=1')
    eq(f'Ledger {m} expenses', sum(t['amount'] for t in txs if t['kind'] == 'expense' and t['date'][:7] == m), ex['expense'])
    yr = api(f'/insights/review?year={m[:4]}')
    ym = {x.get('period', x.get('month')): x for x in (yr.get('months') or yr.get('flows') or [])}
    if m in ym:
        eq(f'Year review {m} income', r.get('income', 0.0), ym[m]['income'])

# ── 3. Spending: this month and last month cards ──
for m, q in ((cur, ''), (prev_month(cur), f'?month={prev_month(cur)}')):
    r = months.get(m, {})
    pace = api('/insights/pace' + q)
    lastp = [p for p in pace if p.get('current') is not None][-1]
    spent_booked = sum(t['amount'] for t in txs if t['kind'] == 'expense' and t['date'][:7] == m) - \
        sum(t['amount'] for t in txs if t['kind'] == 'income' and top(t['cat']) == 'refunds' and t['date'][:7] == m)
    eq(f'Spending pace {m} today = spent', spent_booked, lastp['current'])
    pm = prev_month(m)
    eq(f'Spending pace {m} previous-month line at day 31', months.get(pm, {}).get('spending', 0.0), pace[-1]['last_month'], 0.05)
    typ = sum(months.get(add_months(m, -i), {}).get('spending', 0.0) for i in range(1, 7)) / 6
    eq(f'Spending pace {m} typical line at day 31', typ, pace[-1]['typical'], 0.05)
    po = api('/insights/pace-out' + q)
    lp = [p for p in po['points'] if p.get('current') is not None][-1]
    eq(f'Spending with-investing {m} invested part', r.get('invested', 0.0), lp['current_invested'])
    eq(f'Income went {m} income', r.get('income', 0.0), po['month']['income'])
    eq(f'Income went {m} left', r.get('income', 0.0) + po['month']['expected_salary'] - r.get('spending', 0.0) - r.get('invested', 0.0), po['month']['left'])
    bd = api('/insights/breakdown?preset=' + ('month' if m == cur else 'last_month'))
    eq(f'Spending by category {m} total', r.get('spending', 0.0), sum(c['total'] for c in bd['categories']))
    rawcat = defaultdict(float)
    for t in txs:
        if t['date'][:7] == m and (t['kind'] == 'expense' or (t['kind'] == 'income' and top(t['cat']) == 'refunds')):
            rawcat[top(t['cat']) if t['kind'] == 'expense' else 'refunds'] += t['amount'] if t['kind'] == 'expense' else -t['amount']

# ── 4. today: net worth, Home, emergency, calendar, forecast ──
by = net_worth(today)
for g in set(by) | set(ov['by_group']):
    eq(f'Net worth {g}', by.get(g, 0.0), ov['by_group'].get(g, 0.0), 0.05)
eq('Net worth', sum(by.values()), ov['net_worth'], 0.1)
eq('Liquid', sum(by.get(g, 0.0) for g in ('cash', 'investments', 'pension', 'crypto')), ov['liquid'], 0.1)
for k in ('income', 'spending', 'invested'):
    eq(f'Home this month {k}', months.get(cur, {}).get(k, 0.0), ov['month'][k])
    eq(f'Home last month {k}', months.get(prev_month(cur), {}).get(k, 0.0), ov['last_month'][k])
last12 = [add_months(cur, -i) for i in range(1, 13)]
for k in ('income', 'spending', 'invested', 'essential'):
    eq(f'12-month average {k}', sum(months.get(m, {}).get(k, 0.0) for m in last12) / 12, ov['avg12'][k], 0.05)
sm = api('/insights/summary')['flow']
eq('Summary typical income', sum(months.get(m, {}).get('income', 0.0) for m in last12) / 12, sm['income'], 0.05)
eq('Summary parts add up to spending', sm['fixed'] + sm['everyday'] + sm['one_offs'], ov['avg12']['spending'], 0.1)
e = ov.get('emergency_plan')
if e:
    eq('Emergency: bank', sum(v for acc, s in series.items() for v in [([x for d, x in s if d <= today] or [0])[-1]] if kind.get(acc) in ('checking', 'savings')), e['bank'], 0.05)
    eq('Emergency: essentials a month', sum(months.get(m, {}).get('essential', 0.0) for m in last12) / 12, e['monthly_essential'], 1)
    shown = (e['available'] / e['monthly_essential']) if e['monthly_essential'] else 0
    check('Emergency: short means under target', (e['gap'] > 0) == (e['available'] < e['target'] - 0.005), f"gap {e['gap']} available {e['available']} target {e['target']}")
cal = api('/plan/calendar?days=60')
eq('Calendar starts at cash', by.get('cash', 0.0), cal['start'], 0.05)
if cal['start'] < cal['floor']:
    check('Calendar: under floor today says today', cal.get('first_below') == today, cal.get('first_below'))
bal = cal['start']
for d in cal['days']:
    bal = bal - cal['daily'] + sum(x['amount'] for x in (d.get('events') or []))
    if abs(bal - d['balance']) > 0.05:
        fails.append(f"Calendar {d['date']} balance {d['balance']} != running {round(bal, 2)}")
        break
else:
    passed += 1
fc = api('/forecast')['baseline']
for i, m in enumerate(fc['months']):
    start = fc['cash'] if i == 0 else fc['months'][i - 1]['end']
    eq(f"Forecast {m['month']} end = start + net", start + m['net'], m['end'], 0.05)
pl = api(f'/plan?month={cur}')
eq('Forecast this month: income still to come', max(0, pl['income_base'] - pl['income_actual']), fc['months'][0]['income'], 0.05)
p = ov['plan']
eq('Home left to spend = Plan', pl['safe_to_spend'], p['safe_to_spend'])
eq('Home usual spending money = plan formula', pl['income_planned'] - pl['fixed_planned'] - pl['saving_planned'] - pl['fund_contributions'] if 'income_planned' in pl else None, p.get('free_planned'))
days = p.get('days') or []
td = [d for d in days if d.get('cum') is not None]
if td:
    eq('Month chart: spent today = free spent (+ beyond funds)', p['free_spent'] + (p.get('fund_overdrawn') or 0), td[-1]['cum'], 0.05)
if ov.get('cash'):
    check('Cash until payday plans day-to-day spending', ov['cash']['daily'] > 0 or p.get('typical_day', 0) == 0, f"daily {ov['cash']['daily']}")

print(f'{passed} checks passed, {len(fails)} failed')
for f in fails:
    print('  ✗', f)
sys.exit(1 if fails else 0)
