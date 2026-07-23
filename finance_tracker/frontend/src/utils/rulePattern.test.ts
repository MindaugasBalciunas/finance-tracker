import { describe, it, expect } from 'vitest'
import { deriveRulePattern, ruleCoversComment } from './rulePattern'

describe('deriveRulePattern', () => {
  it('takes the merchant segment before punctuation', () => {
    expect(deriveRulePattern('Pammukale. Kebab')).toBe('pammukale')
    expect(deriveRulePattern('Neste. full tank')).toBe('neste')
    expect(deriveRulePattern('Skonio receptai (Date)')).toBe('skonio receptai')
  })

  it('strips legal-form prefixes', () => {
    expect(deriveRulePattern('UAB SAUGOS TARNYBA ARGUS')).toBe('saugos tarnyba')
    expect(deriveRulePattern('AB VILNIAUS TINKLAI')).toBe('vilniaus tinklai')
  })

  it('stops at numeric noise', () => {
    expect(deriveRulePattern('McDonalds 44000017 LT-04352 Vilnius')).toBe('mcdonalds')
    expect(deriveRulePattern('BOLT.EU/O/2407191413 10134 Tallinn')).toBe('bolt')
  })

  it('caps at two words', () => {
    expect(deriveRulePattern('Devyni drakonai penki Vilnius')).toBe('devyni drakonai')
  })

  it('returns empty for all-noise comments', () => {
    expect(deriveRulePattern('123456')).toBe('')
    expect(deriveRulePattern('')).toBe('')
  })
})

describe('ruleCoversComment', () => {
  const rules = [
    { label: 'fuel', category: '', comment_match: 'neste' },
    { label: 'kristina', category: 'Dating', comment_match: '' },
  ]

  it('detects comment-match coverage', () => {
    expect(ruleCoversComment(rules, 'fuel', 'Neste. full tank', 'Transport')).toBe(true)
    expect(ruleCoversComment(rules, 'fuel', 'Circle K', 'Transport')).toBe(false)
  })

  it('detects category coverage', () => {
    expect(ruleCoversComment(rules, 'kristina', 'anything', 'Dating')).toBe(true)
    expect(ruleCoversComment(rules, 'kristina', 'anything', 'Food')).toBe(false)
  })

  it('unknown label is never covered', () => {
    expect(ruleCoversComment(rules, 'security', 'Argus (security)', 'Utilities')).toBe(false)
  })
})
