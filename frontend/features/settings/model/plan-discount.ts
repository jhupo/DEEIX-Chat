export function resolvePlanDiscountPercent(originalPriceCents: number, priceCents: number): number | null {
  if (!Number.isFinite(originalPriceCents) || !Number.isFinite(priceCents)) {
    return null;
  }
  if (originalPriceCents <= 0 || priceCents < 0 || priceCents >= originalPriceCents) {
    return null;
  }
  return Math.max(1, Math.round((1 - priceCents / originalPriceCents) * 100));
}
