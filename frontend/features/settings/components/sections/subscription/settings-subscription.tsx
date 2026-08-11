"use client";

import * as React from "react";
import dynamic from "next/dynamic";
import { useTranslations } from "next-intl";
import { toast } from "sonner";
import { RefreshCw } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { useAppLocale } from "@/i18n/app-i18n-provider";
import { useLocalizedErrorMessage } from "@/i18n/use-localized-error";
import { useAuthSession } from "@/shared/auth/auth-session-context";
import { createIdempotencyKey } from "@/shared/lib/idempotency-key";
import {
  createBillingCheckout,
  getBillingConfig,
  getBillingOverview,
  listBillingDailyUsage,
  listBillingMonthlyUsage,
  listBillingUsage,
  redeemBillingCode,
  verifyBillingOrder,
} from "@/shared/api/billing";
import type {
  BillingConfigData,
  BillingMode,
  BillingOverviewData,
  BillingUsageDailyDTO,
  BillingUsageLedgerDTO,
  BillingUsageMonthlyDTO,
  BillingUsageSort,
  BillingUsageType,
} from "@/shared/api/billing.types";
import type { BillingPlanDTO, BillingPlanPriceDTO } from "@/shared/api/billing.types";
import { SettingsPage, SettingsSectionHeader } from "@/shared/components/settings-layout";
import {
  formatAccountBalance,
  isFreePlan,
  planRank,
  resolveDefaultPrice,
  resolvePlanActionKind,
  billingDisplayAmountToMinorUnits,
  billingDisplayAmountToUSD,
} from "@/features/settings/model/subscription-format";
import {
  normalizeBillingDisplayCurrency,
  type BillingDisplayOptions,
} from "@/shared/lib/billing-display";
import { RedemptionDialog, TopUpDialog } from "./subscription-billing-dialogs";
import { SubscriptionSummary } from "./subscription-summary";
import { SubscriptionUsageLog } from "./subscription-usage-log";
import type { UsageTrendView } from "./subscription-trend";

const SubscriptionTrend = dynamic(
  () => import("./subscription-trend").then((module) => module.SubscriptionTrend),
  {
    ssr: false,
    loading: () => <SubscriptionTrendSkeleton />,
  },
);

function SubscriptionTrendSkeleton() {
  return (
    <div className="space-y-4">
      <div className="flex h-9 items-center justify-between gap-3">
        <div className="h-4 w-28 rounded-full bg-muted/50" />
        <div className="h-7 w-24 rounded-full bg-muted/50" />
      </div>
      <div className="grid grid-cols-2 gap-2 text-xs md:grid-cols-4">
        {Array.from({ length: 4 }).map((_, index) => (
          <div key={`subscription-trend-skeleton-${index}`} className="rounded-md bg-muted/40 p-3">
            <div className="h-3 w-16 rounded-full bg-muted/60" />
            <div className="mt-2 h-4 w-20 rounded-full bg-muted/60" />
          </div>
        ))}
      </div>
      <div className="rounded-md bg-muted/35 p-3">
        <div className="h-[260px] rounded-md bg-muted/30" />
      </div>
    </div>
  );
}

type BillingRuntimeConfig = BillingConfigData["config"];
type PaymentProvider = string;

function paymentReturnURL(operationID: string, state: "success" | "cancel"): string {
  const url = new URL("/setting/subscription", window.location.origin);
  url.searchParams.set("payment", state);
  url.searchParams.set("operation", operationID);
  return url.toString();
}

export function SettingsSubscription() {
  const t = useTranslations("settings.subscriptionPage");
  const resolveErrorMessage = useLocalizedErrorMessage();
  const { locale } = useAppLocale();
  const { accessToken } = useAuthSession();
  const [billingPlans, setBillingPlans] = React.useState<BillingPlanDTO[]>([]);
  const [billingConfig, setBillingConfig] = React.useState<BillingRuntimeConfig | null>(null);
  const [billingOverview, setBillingOverview] = React.useState<BillingOverviewData["overview"] | null>(null);
  const [usageLedgers, setUsageLedgers] = React.useState<BillingUsageLedgerDTO[]>([]);
  const [dailyUsage, setDailyUsage] = React.useState<BillingUsageDailyDTO[]>([]);
  const [monthlyUsage, setMonthlyUsage] = React.useState<BillingUsageMonthlyDTO[]>([]);
  const [usageTotal, setUsageTotal] = React.useState(0);
  const [usagePage, setUsagePage] = React.useState(1);
  const [usagePageSize, setUsagePageSize] = React.useState(25);
  const [usageQuery, setUsageQuery] = React.useState("");
  const [usageBillingType, setUsageBillingType] = React.useState<BillingUsageType | "">("");
  const [usageSort, setUsageSort] = React.useState<BillingUsageSort>("newest");
  const [usageView, setUsageView] = React.useState<UsageTrendView>("daily");
  const [billingLoading, setBillingLoading] = React.useState(true);
  const [usageLoading, setUsageLoading] = React.useState(true);
  const [checkoutPriceID, setCheckoutPriceID] = React.useState<number | null>(null);
  const [topUpAmount, setTopUpAmount] = React.useState("20");
  const [topUpLoading, setTopUpLoading] = React.useState(false);
  const [pricingDialogOpen, setPricingDialogOpen] = React.useState(false);
  const [paymentDialogOpen, setPaymentDialogOpen] = React.useState(false);
  const [selectedPlan, setSelectedPlan] = React.useState<BillingPlanDTO | null>(null);
  const [selectedPrice, setSelectedPrice] = React.useState<BillingPlanPriceDTO | null>(null);
  const [selectedPaymentProvider, setSelectedPaymentProvider] = React.useState<PaymentProvider>("");
  const [topUpDialogOpen, setTopUpDialogOpen] = React.useState(false);
  const [redemptionDialogOpen, setRedemptionDialogOpen] = React.useState(false);
  const [redemptionCode, setRedemptionCode] = React.useState("");
  const [redemptionLoading, setRedemptionLoading] = React.useState(false);
  const paymentVerificationRef = React.useRef("");
  const billingMode: BillingMode = billingConfig?.mode ?? "usage";
  const billingDisplay = React.useMemo<BillingDisplayOptions>(
    () => ({
      currency: normalizeBillingDisplayCurrency(billingConfig?.displayCurrency),
      usdToCnyRate: billingConfig?.usdToCNYRate ?? null,
    }),
    [billingConfig?.displayCurrency, billingConfig?.usdToCNYRate],
  );

  const intervalLabels = React.useMemo(
    () => ({
      lifetime: t("interval.lifetime"),
      year: t("interval.year"),
      month: t("interval.month"),
    }),
    [t],
  );
  const planActionLabels = React.useMemo(
    () => ({
      current: t("plans.actions.current"),
      unavailable: t("plans.actions.unavailable"),
      renew: t("plans.actions.renew"),
      subscribe: t("plans.actions.subscribe"),
      switch: t("plans.actions.switch"),
      upgrade: t("plans.actions.upgrade"),
      freeBlocked: t("plans.actions.freeBlocked"),
    }),
    [t],
  );
  const planFeatureLabels = React.useMemo(
    () => ({
      monthlyCredit: (credit: string) => t("plans.features.monthlyCredit", { credit }),
      freeModelsNotIncluded: t("plans.features.freeModelsNotIncluded"),
    }),
    [t],
  );
  const entitlementLabels = React.useMemo(
    () => ({
      title: t("entitlements.title"),
      count: (count: number) => t("entitlements.count", { count }),
      current: t("entitlements.current"),
      upcoming: t("entitlements.upcoming"),
      range: (start: string, end: string) => t("entitlements.range", { start, end }),
      credit: (credit: string) => t("entitlements.credit", { credit }),
    }),
    [t],
  );
  const loadBillingData = React.useCallback(async () => {
    setBillingLoading(true);
    try {
      const [configData, overviewData, nextDailyUsage, nextMonthlyUsage] = await Promise.all([
        getBillingConfig(accessToken),
        getBillingOverview(accessToken),
        listBillingDailyUsage(accessToken),
        listBillingMonthlyUsage(accessToken, 12),
      ]);
      setBillingConfig(configData.config);
      setBillingPlans(configData.config.plans);
      setBillingOverview(overviewData.overview);
      setDailyUsage(nextDailyUsage ?? []);
      setMonthlyUsage(nextMonthlyUsage ?? []);
    } catch (error) {
      toast.error(t("toasts.subscriptionLoadFailed"), { description: resolveErrorMessage(error, t("toasts.retryLater")) });
    } finally {
      setBillingLoading(false);
    }
  }, [accessToken, resolveErrorMessage, t]);

  React.useEffect(() => {
    void loadBillingData();
  }, [loadBillingData]);

  const loadUsageLogs = React.useCallback(async (page: number, pageSize: number, query: string, billingType: BillingUsageType | "", sort: BillingUsageSort) => {
    setUsageLoading(true);
    try {
      const usage = await listBillingUsage(accessToken, { page, pageSize, query, billingType: billingType || undefined, sort });
      setUsageLedgers(usage.results ?? []);
      setUsageTotal(usage.total ?? 0);
    } catch (error) {
      toast.error(t("toasts.usageLogLoadFailed"), { description: resolveErrorMessage(error, t("toasts.retryLater")) });
    } finally {
      setUsageLoading(false);
    }
  }, [accessToken, resolveErrorMessage, t]);

  React.useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    const state = params.get("payment");
    const operationID = params.get("operation") ?? "";
    if ((state !== "success" && state !== "cancel") || !/^[0-9a-f-]{36}$/.test(operationID)) return;

    if (state === "cancel") {
      window.history.replaceState({}, "", window.location.pathname);
      return;
    }
    if (paymentVerificationRef.current === operationID) return;
    paymentVerificationRef.current = operationID;

    void verifyBillingOrder(accessToken, operationID)
      .then(async () => {
        const [overview, nextDailyUsage, nextMonthlyUsage] = await Promise.all([
          getBillingOverview(accessToken),
          listBillingDailyUsage(accessToken),
          listBillingMonthlyUsage(accessToken, 12),
        ]);
        setBillingOverview(overview.overview);
        setDailyUsage(nextDailyUsage ?? []);
        setMonthlyUsage(nextMonthlyUsage ?? []);
        await loadUsageLogs(1, 25, "", "", "newest");
        window.history.replaceState({}, "", window.location.pathname);
        toast.success(t("toasts.paymentVerified"));
      })
      .catch((error) => {
        toast.error(t("toasts.paymentVerifyFailed"), { description: resolveErrorMessage(error, t("toasts.retryLater")) });
      });
  }, [accessToken, loadUsageLogs, resolveErrorMessage, t]);

  React.useEffect(() => {
    void loadUsageLogs(usagePage, usagePageSize, usageQuery, usageBillingType, usageSort);
  }, [loadUsageLogs, usageBillingType, usagePage, usagePageSize, usageQuery, usageSort]);

  const paymentProviders = React.useMemo(() => billingConfig?.paymentMethods.map((item) => item.id).filter((item) => item.trim()) ?? [], [billingConfig?.paymentMethods]);

  React.useEffect(() => {
    if (paymentProviders.length > 0 && !paymentProviders.includes(selectedPaymentProvider)) {
      setSelectedPaymentProvider(paymentProviders[0] ?? "");
    }
  }, [paymentProviders, selectedPaymentProvider]);

  const handleCheckout = React.useCallback(async (price: BillingPlanPriceDTO, paymentProvider: PaymentProvider) => {
    setCheckoutPriceID(price.id);
    try {
      const operationID = createIdempotencyKey();
      const data = await createBillingCheckout(accessToken, {
        orderType: "subscription",
        priceID: price.id,
        cycles: 1,
        paymentProvider,
        successURL: paymentReturnURL(operationID, "success"),
        cancelURL: paymentReturnURL(operationID, "cancel"),
      }, operationID);
      if (!data.checkout.checkoutURL) {
        toast.error(t("toasts.checkoutCreateFailed"), { description: t("toasts.checkoutURLMissing") });
        return;
      }
      window.open(data.checkout.checkoutURL, "_blank", "noopener,noreferrer");
    } catch (error) {
      toast.error(t("toasts.checkoutCreateFailed"), { description: resolveErrorMessage(error, t("toasts.retryLater")) });
    } finally {
      setCheckoutPriceID(null);
    }
  }, [accessToken, resolveErrorMessage, t]);

  const handleTopUp = React.useCallback(async () => {
    const displayAmount = Number(topUpAmount);
    const amountMinorUnits = billingDisplayAmountToMinorUnits(billingDisplayAmountToUSD(displayAmount, billingDisplay));
    if (!Number.isFinite(displayAmount) || displayAmount <= 0 || amountMinorUnits <= 0) {
      toast.error(t("toasts.invalidTopUpAmount"), { description: t("toasts.invalidTopUpAmountDescription") });
      return;
    }
    setTopUpLoading(true);
    try {
      const operationID = createIdempotencyKey();
      const data = await createBillingCheckout(accessToken, {
        orderType: "topup",
        amountMinorUnits,
        cycles: 1,
        paymentProvider: selectedPaymentProvider,
        successURL: paymentReturnURL(operationID, "success"),
        cancelURL: paymentReturnURL(operationID, "cancel"),
      }, operationID);
      if (!data.checkout.checkoutURL) {
        toast.error(t("toasts.checkoutCreateFailed"), { description: t("toasts.checkoutURLMissing") });
        return;
      }
      window.open(data.checkout.checkoutURL, "_blank", "noopener,noreferrer");
    } catch (error) {
      toast.error(t("toasts.checkoutCreateFailed"), { description: resolveErrorMessage(error, t("toasts.retryLater")) });
    } finally {
      setTopUpLoading(false);
    }
  }, [accessToken, billingDisplay, resolveErrorMessage, selectedPaymentProvider, t, topUpAmount]);

  const handleRedeemCode = React.useCallback(async () => {
    const code = redemptionCode.trim();
    if (!code) {
      toast.error(t("toasts.invalidRedemptionCode"));
      return;
    }
    setRedemptionLoading(true);
    try {
      const data = await redeemBillingCode(accessToken, { code });
      setBillingOverview(data.overview);
      setRedemptionDialogOpen(false);
      setRedemptionCode("");
      toast.success(t("toasts.redemptionSucceeded"));
    } catch (error) {
      toast.error(t("toasts.redemptionFailed"), { description: resolveErrorMessage(error, t("toasts.retryLater")) });
    } finally {
      setRedemptionLoading(false);
    }
  }, [accessToken, redemptionCode, resolveErrorMessage, t]);

  const subscriptionEntitlements = React.useMemo(
    () => billingOverview?.subscriptionEntitlements ?? [],
    [billingOverview?.subscriptionEntitlements],
  );
  const paymentDisabled = paymentProviders.length === 0;
  const currentPlan = billingOverview?.plan ?? null;
  const currentPrice = React.useMemo(() => resolveDefaultPrice(currentPlan), [currentPlan]);
  const protectedPaidPlanRank = React.useMemo(
    () => Math.max(
      currentPlan && !isFreePlan(currentPlan) ? planRank(currentPlan) : 0,
      ...subscriptionEntitlements.map((item) => isFreePlan(item.plan) ? 0 : planRank(item.plan)),
    ),
    [currentPlan, subscriptionEntitlements],
  );

  const handleSelectPlan = React.useCallback(
    async (plan: BillingPlanDTO, price: BillingPlanPriceDTO | null, isCurrent: boolean) => {
      if (isCurrent && isFreePlan(plan)) {
        return;
      }
      if (!price) {
        toast.error(t("toasts.planUnavailable"), { description: t("toasts.planUnavailableDescription") });
        return;
      }
      const actionKind = resolvePlanActionKind(
        plan,
        price,
        isCurrent,
        currentPlan,
        protectedPaidPlanRank,
      );
      if (actionKind === "freeBlocked") {
        toast.error(t("toasts.freeSwitchBlocked"), { description: t("toasts.freeSwitchBlockedDescription") });
        return;
      }
      if (price.amountCents > 0) {
        if (paymentDisabled) {
          toast.error(t("toasts.paymentDisabled"), { description: t("toasts.paymentDisabledDescription") });
          return;
        }
        setSelectedPlan(plan);
        setSelectedPrice(price);
        setPricingDialogOpen(false);
        setPaymentDialogOpen(true);
        return;
      }
      toast.error(t("toasts.planUnavailable"), { description: t("toasts.planUnavailableDescription") });
    },
    [currentPlan, paymentDisabled, protectedPaidPlanRank, t],
  );

  const handleConfirmPayment = React.useCallback(async () => {
    if (!selectedPrice) {
      toast.error(t("toasts.noPlanSelected"), { description: t("toasts.noPlanSelectedDescription") });
      return;
    }
    await handleCheckout(selectedPrice, selectedPaymentProvider);
  }, [handleCheckout, selectedPaymentProvider, selectedPrice, t]);

  const periodCredit = billingOverview?.periodCreditUSD ?? currentPlan?.monthlyLimitUSD ?? 0;
  const periodUsed = billingOverview?.periodUsedUSD ?? 0;
  const periodPercent = periodCredit > 0 ? Math.min(100, Math.max(0, (periodUsed / periodCredit) * 100)) : 0;
  const billingAccount = billingOverview?.account ?? null;

  return (
    <SettingsPage className="space-y-6">
      <SettingsSectionHeader
        title={t("title")}
        className="px-1"
        actions={(
          <Tooltip>
            <TooltipTrigger asChild>
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                disabled={billingLoading || usageLoading}
                onClick={() => void Promise.all([
                  loadBillingData(),
                  loadUsageLogs(usagePage, usagePageSize, usageQuery, usageBillingType, usageSort),
                ])}
                aria-label={t("refresh")}
              >
                <RefreshCw className={billingLoading || usageLoading ? "animate-spin" : ""} />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t("refresh")}</TooltipContent>
          </Tooltip>
        )}
      />

      <SubscriptionSummary
        billingMode={billingMode}
        billingLoading={billingLoading}
        redemptionLoading={redemptionLoading}
        topUpLoading={topUpLoading}
        paymentDisabled={paymentDisabled}
        billingPlans={billingPlans}
        billingOverview={billingOverview}
        currentPlan={currentPlan}
        currentPrice={currentPrice}
        billingAccount={billingAccount}
        subscriptionEntitlements={subscriptionEntitlements}
        locale={locale}
        intervalLabels={intervalLabels}
        entitlementLabels={entitlementLabels}
        planActionLabels={planActionLabels}
        planFeatureLabels={planFeatureLabels}
        paymentProviders={paymentProviders}
        selectedPlan={selectedPlan}
        selectedPrice={selectedPrice}
        selectedPaymentProvider={selectedPaymentProvider}
        checkoutPriceID={checkoutPriceID}
        pricingDialogOpen={pricingDialogOpen}
        paymentDialogOpen={paymentDialogOpen}
        protectedPaidPlanRank={protectedPaidPlanRank}
        periodCredit={periodCredit}
        periodUsed={periodUsed}
        periodPercent={periodPercent}
        billingDisplay={billingDisplay}
        onOpenRedemptionDialog={() => setRedemptionDialogOpen(true)}
        onOpenTopUpDialog={() => setTopUpDialogOpen(true)}
        onPricingDialogOpenChange={setPricingDialogOpen}
        onPaymentDialogOpenChange={setPaymentDialogOpen}
        onSelectPlan={(plan, price, isCurrent) => void handleSelectPlan(plan, price, isCurrent)}
        onPaymentProviderChange={setSelectedPaymentProvider}
        onConfirmPayment={() => void handleConfirmPayment()}
      />

      <section className="space-y-6 px-0.5 md:space-y-7 xl:space-y-8 xl:px-1">
        <Separator />
        <SubscriptionTrend
          dailyUsage={dailyUsage}
          monthlyUsage={monthlyUsage}
          loading={billingLoading}
          view={usageView}
          billingDisplay={billingDisplay}
          onViewChange={setUsageView}
        />
        <Separator />
        <SubscriptionUsageLog
          items={usageLedgers}
          total={usageTotal}
          loading={usageLoading}
          page={usagePage}
          pageSize={usagePageSize}
          query={usageQuery}
          billingType={usageBillingType}
          sort={usageSort}
          billingDisplay={billingDisplay}
          onQueryChange={(value) => {
            setUsageQuery(value);
            setUsagePage(1);
          }}
          onBillingTypeChange={(value) => {
            setUsageBillingType(value);
            setUsagePage(1);
          }}
          onSortChange={(value) => {
            setUsageSort(value);
            setUsagePage(1);
          }}
          onRefresh={() => void loadUsageLogs(usagePage, usagePageSize, usageQuery, usageBillingType, usageSort)}
          onPageChange={setUsagePage}
          onPageSizeChange={(nextPageSize) => {
            setUsagePageSize(nextPageSize);
            setUsagePage(1);
          }}
        />
      </section>

      <TopUpDialog
        open={topUpDialogOpen}
        onOpenChange={setTopUpDialogOpen}
        amount={topUpAmount}
        currentBalance={formatAccountBalance(billingAccount?.balance ?? 0)}
        billingLoading={billingLoading}
        topUpLoading={topUpLoading}
        paymentDisabled={paymentDisabled}
        paymentProviders={paymentProviders}
        paymentMethods={billingConfig?.paymentMethods ?? []}
        selectedPaymentProvider={selectedPaymentProvider}
        billingDisplay={billingDisplay}
        balanceRechargeMultiplier={billingConfig?.balanceRechargeMultiplier ?? 1}
        rechargeFeeRate={billingConfig?.rechargeFeeRate ?? 0}
        onAmountChange={setTopUpAmount}
        onPaymentProviderChange={setSelectedPaymentProvider}
        onSubmit={() => void handleTopUp()}
      />

      <RedemptionDialog
        open={redemptionDialogOpen}
        onOpenChange={setRedemptionDialogOpen}
        code={redemptionCode}
        billingLoading={billingLoading}
        redemptionLoading={redemptionLoading}
        onCodeChange={setRedemptionCode}
        onSubmit={() => void handleRedeemCode()}
      />
    </SettingsPage>
  );
}
