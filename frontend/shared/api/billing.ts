import { authedRequest } from "@/shared/api/authed-client";
import type { PagePayload } from "@/shared/api/common.types";
import type {
  BillingAccountData,
  BillingConfigData,
  BillingOverviewData,
  BillingOrderData,
  BillingUsageDailyDTO,
  BillingUsageHourlyDTO,
  BillingUsageSort,
  BillingUsageType,
  BillingPlanDTO,
  BillingUsageLedgerDTO,
  BillingUsageMonthlyDTO,
  CheckoutData,
  CreateCheckoutRequest,
  RedeemBillingCodeData,
  RedeemBillingCodeRequest,
} from "@/shared/api/billing.types";

export async function getBillingConfig(accessToken: string): Promise<BillingConfigData> {
  return authedRequest<BillingConfigData>("/api/v1/billing/config", { accessToken }, true);
}

export async function listBillingPlans(accessToken: string): Promise<BillingPlanDTO[]> {
  return authedRequest<BillingPlanDTO[]>("/api/v1/billing/plans", { accessToken }, true);
}

export async function getBillingAccount(accessToken: string): Promise<BillingAccountData> {
  return authedRequest<BillingAccountData>("/api/v1/billing/account", { accessToken }, true);
}

export async function getBillingOverview(accessToken: string): Promise<BillingOverviewData> {
  return authedRequest<BillingOverviewData>("/api/v1/billing/overview", { accessToken }, true);
}

export async function listBillingUsage(
  accessToken: string,
  options: { page?: number; pageSize?: number; query?: string; billingType?: BillingUsageType; sort?: BillingUsageSort } = {},
): Promise<PagePayload<BillingUsageLedgerDTO>> {
  const page = options.page && options.page > 0 ? options.page : 1;
  const pageSize = options.pageSize && options.pageSize > 0 ? options.pageSize : 10;
  const params = new URLSearchParams({
    page: String(page),
    page_size: String(pageSize),
  });
  if (options.query?.trim()) params.set("query", options.query.trim());
  if (options.billingType) params.set("billing_type", options.billingType);
  params.set("sort_by", "created_at");
  params.set("sort_order", options.sort === "oldest" ? "asc" : "desc");
  return authedRequest<PagePayload<BillingUsageLedgerDTO>>(
    `/api/v1/billing/usage?${params.toString()}`,
    { accessToken },
    true,
  );
}

export async function listBillingMonthlyUsage(accessToken: string, months = 12): Promise<BillingUsageMonthlyDTO[]> {
  const params = new URLSearchParams({ months: String(months) });
  const data = await authedRequest<{ results: BillingUsageMonthlyDTO[] }>(
    `/api/v1/billing/usage/monthly?${params.toString()}`,
    { accessToken },
    true,
  );
  return data.results;
}

export async function listBillingDailyUsage(
  accessToken: string,
  options: { days?: number; startDate?: string; endDate?: string } = {},
): Promise<BillingUsageDailyDTO[]> {
  const params = new URLSearchParams();
  if (options.startDate && options.endDate) {
    params.set("start_date", options.startDate);
    params.set("end_date", options.endDate);
  } else if (options.days && options.days > 0) {
    params.set("days", String(options.days && options.days > 0 ? options.days : 30));
  }
  const query = params.toString();
  const data = await authedRequest<{ results: BillingUsageDailyDTO[] }>(
    `/api/v1/billing/usage/daily${query ? `?${query}` : ""}`,
    { accessToken },
    true,
  );
  return data.results;
}

export async function listBillingHourlyUsage(accessToken: string, days = 1): Promise<BillingUsageHourlyDTO[]> {
  const params = new URLSearchParams({ days: String(days) });
  const data = await authedRequest<{ results: BillingUsageHourlyDTO[] }>(
    `/api/v1/billing/usage/hourly?${params.toString()}`,
    { accessToken },
    true,
  );
  return data.results;
}

export async function createBillingCheckout(accessToken: string, payload: CreateCheckoutRequest, idempotencyKey: string): Promise<CheckoutData> {
  return authedRequest<CheckoutData>(
    "/api/v1/billing/payments/checkout",
    { method: "POST", accessToken, body: payload, headers: { "Idempotency-Key": idempotencyKey } },
    true,
  );
}

export async function redeemBillingCode(accessToken: string, payload: RedeemBillingCodeRequest): Promise<RedeemBillingCodeData> {
  return authedRequest<RedeemBillingCodeData>(
    "/api/v1/billing/redemptions",
    { method: "POST", accessToken, body: payload },
    true,
  );
}

export async function verifyBillingOrder(accessToken: string, operationID: string): Promise<BillingOrderData> {
  return authedRequest<BillingOrderData>(
    "/api/v1/billing/orders/verify",
    { method: "POST", accessToken, body: { operationID } },
    true,
  );
}
