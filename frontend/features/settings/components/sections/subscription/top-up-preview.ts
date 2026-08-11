export type TopUpPreview = {
  paymentAmount: number;
  creditedUSD: number;
};

export function calculateTopUpPreview(baseUSD: number, paymentCurrency: string, usdToCNYRate: number | null | undefined, balanceRechargeMultiplier: number, rechargeFeeRate: number): TopUpPreview {
  const amount = Number.isFinite(baseUSD) && baseUSD > 0 ? baseUSD : 0;
  const multiplier = Number.isFinite(balanceRechargeMultiplier) && balanceRechargeMultiplier > 0 ? balanceRechargeMultiplier : 1;
  const fee = Number.isFinite(rechargeFeeRate) && rechargeFeeRate > 0 ? rechargeFeeRate : 0;
  const currency = paymentCurrency.trim().toUpperCase();
  const converted = currency === "CNY" && Number.isFinite(usdToCNYRate) && (usdToCNYRate ?? 0) > 0 ? amount * Number(usdToCNYRate) : amount;
  return { paymentAmount: converted * (1 + fee), creditedUSD: amount * multiplier };
}
