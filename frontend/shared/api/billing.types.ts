export type BillingMode = "period" | "usage";
export type BillingUsageType = "balance" | "subscription";
export type BillingUsageSort = "newest" | "oldest";

export type BillingPlanPriceDTO = {
  id: number;
  planID: number;
  code: string;
  billingInterval: "month" | "year" | "lifetime";
  currency: string;
  amountCents: number;
  isActive: boolean;
  isDefault: boolean;
};

export type BillingPlanDTO = {
  id: number;
  code: string;
  name: string;
  description: string;
  featureJSON: string;
  groupPlatform: string;
  rateMultiplier: number;
  modelRateMultiplier: number;
  dailyLimitUSD: number | null;
  weeklyLimitUSD: number | null;
  monthlyLimitUSD: number | null;
  validityDays: number;
  originalPriceCents: number;
  modelScopesJSON: string;
  periodCreditUSD: number;
  sortOrder: number;
  isActive: boolean;
  prices: BillingPlanPriceDTO[];
};

export type BillingPaymentMethodDTO = {
  id: string;
  currency: string;
  min: number;
  max: number;
};

export type BillingConfigData = {
  config: {
    mode: BillingMode;
    paymentMethods: BillingPaymentMethodDTO[];
    displayCurrency: string;
    usdToCNYRate: number;
    balanceDisabled: boolean;
    balanceRechargeMultiplier: number;
    rechargeFeeRate: number;
    globalDailyLimitUSD: number | null;
    globalWeeklyLimitUSD: number | null;
    globalMonthlyLimitUSD: number | null;
    plans: BillingPlanDTO[];
  };
  observedAt: string;
};

export type BillingAccountData = {
  account: {
    balance: number;
    frozenBalance: number;
    status: string;
  };
  observedAt: string;
};

export type BillingSubscriptionEntitlementDTO = {
  id: number;
  userID: number;
  planID: number;
  priceID: number;
  status: string;
  startAt: string;
  currentPeriodStartAt: string;
  currentPeriodEndAt: string;
  cancelAtPeriodEnd: boolean;
  autoRenew: boolean;
  plan: BillingPlanDTO;
  isCurrent: boolean;
};

export type BillingOverviewData = {
  overview: {
    mode: BillingMode;
    account: BillingAccountData["account"];
    plan: BillingPlanDTO | null;
    periodStartAt: string | null;
    periodEndAt: string | null;
    periodCreditUSD: number;
    periodCreditNanousd: number;
    periodUsedUSD: number;
    periodUsedNanousd: number;
    periodRemainingUSD: number;
    periodRemainingNanousd: number;
    subscriptionEntitlements: BillingSubscriptionEntitlementDTO[];
  };
  observedAt: string;
};

export type BillingUsageLedgerDTO = {
  id: number;
  model: string;
  inputTokens: number;
  outputTokens: number;
  totalTokens: number;
  actualCost: string;
  durationMS: number;
  createdAt: string;
};

export type BillingUsageDailyDTO = {
  usageDate: string;
  callCount: number;
  recordCount: number;
  inputTokens: number;
  outputTokens: number;
  cacheReadTokens: number;
  cacheWriteTokens: number;
  totalTokens: number;
  actualCost: string;
};

export type BillingUsageHourlyDTO = Omit<BillingUsageDailyDTO, "usageDate"> & {
  bucketStart: string;
};

export type BillingUsageMonthlyDTO = {
  monthStartAt: string;
  callCount: number;
  recordCount: number;
  inputTokens: number;
  outputTokens: number;
  cacheReadTokens: number;
  cacheWriteTokens: number;
  totalTokens: number;
  actualCost: string;
};

export type CreateCheckoutRequest = {
  orderType: "subscription" | "topup";
  priceID?: number;
  amountMinorUnits?: number;
  paymentProvider: string;
  cycles?: number;
};

export type CheckoutData = {
  checkout: {
    orderNo: string;
    orderType: string;
    provider: string;
    status: string;
    externalCheckoutID: string;
    checkoutURL: string;
    qrCode: string;
    baseAmountCents: number;
    baseCurrency: string;
    payAmountCents: number;
    payCurrency: string;
    fxRate: string;
    creditNanousd: number;
    creditUSD: number;
    expiredAt: string | null;
  };
  observedAt: string;
};

export type BillingOrderDTO = {
  id: number;
  orderNo: string;
  orderType: string;
  status: string;
  amountUSD: number;
  payAmount: number;
  currency: string;
  paymentType: string;
  createdAt: string;
  expiresAt: string;
  paidAt: string | null;
  completedAt: string | null;
};

export type BillingOrderData = {
  order: BillingOrderDTO;
  observedAt: string;
};

export type RedeemBillingCodeRequest = { code: string };
export type RedeemBillingCodeData = {
  redemption: { id: number; type: string; value: number };
  account: BillingAccountData["account"];
  overview: BillingOverviewData["overview"];
  observedAt: string;
};
