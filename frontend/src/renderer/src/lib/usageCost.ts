import type { UsageTotals } from './api/types'
import { type ModelPricing, pricingIdForUsage } from './models'
import { inputTokens, totalUsageTokens, type UsageModelTotals } from './usageDaily'

export function buildPricingIndex(models: { value: string; pricing?: ModelPricing }[]): Map<string, ModelPricing> {
  const index = new Map<string, ModelPricing>()
  for (const model of models) {
    if (model.pricing) index.set(model.value, model.pricing)
  }
  return index
}

function estimateUsageCost(usage: UsageTotals, pricing: ModelPricing): number {
  const cacheRead = usage.cached_input_tokens ?? 0
  const cacheWrite = usage.cached_write_tokens ?? 0
  const output = usage.output_tokens ?? 0
  return inputTokens(usage) * pricing.input + cacheRead * pricing.cacheRead + cacheWrite * pricing.cacheWrite + output * pricing.output
}

export interface PricedModel {
  model: UsageModelTotals
  cost: number | null
}

export interface CostSummary {
  total: number
  priced: number
  unpriced: number
}

export function priceModels(
  models: UsageModelTotals[],
  index: Map<string, ModelPricing>,
): { rows: PricedModel[]; summary: CostSummary } {
  const rows: PricedModel[] = []
  const summary: CostSummary = { total: 0, priced: 0, unpriced: 0 }
  for (const model of models) {
    const id = pricingIdForUsage(model)
    const pricing = id ? index.get(id) : undefined
    const cost = pricing ? estimateUsageCost(model.usage, pricing) : null
    rows.push({ model, cost })
    if (cost != null) {
      summary.total += cost
      summary.priced += 1
    } else if (totalUsageTokens(model.usage) > 0) {
      summary.unpriced += 1
    }
  }
  return { rows, summary }
}

export function formatUsd(amount: number): string {
  if (amount <= 0) return '$0'
  if (amount < 0.01) return '<$0.01'
  if (amount < 1000) return `$${amount.toFixed(2)}`
  if (amount < 1_000_000) return `$${(amount / 1000).toFixed(1)}k`
  return `$${(amount / 1_000_000).toFixed(1)}M`
}
