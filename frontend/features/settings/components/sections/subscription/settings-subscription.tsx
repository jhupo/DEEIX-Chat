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
  listBillingHourlyUsage,
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
  BillingUsageHourlyDTO,
  BillingUsageLedgerDTO,
  BillingUsageMonthlyDTO,
  BillingUsageSort,
  BillingUsageType,
  CheckoutData,
} from "@/shared/api/billing.types";
import type { BillingPlanDTO, BillingPlanPriceDTO } from "@/shared/api/billing.types";
import { SettingsPage, SettingsSectionHeader } from "@/shared/components/settings-layout";
import {
  formatAccountBalance,
  isFreePlan,
  planRank,
  resolvePlanActionKind,
  billingDisplayAmountToMinorUnits,
  billingDisplayAmountToUSD,
} from "@/features/settings/model/subscription-format";
import {
  normalizeBillingDisplayCurrency,
  type BillingDisplayOptions,
} from "@/shared/lib/billing-display";
import { PendingPaymentDialog, RedemptionDialog, TopUpDialog } from "./subscription-billing-dialogs";
import { SubscriptionSummary } from "./subscription-summary";
import { SubscriptionAPIKeyDialog } from "./subscription-api-key-dialog";
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
type PendingPayment = CheckoutData["checkout"] & { operationID: string };

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
  const [hourlyUsage, setHourlyUsage] = React.useState<BillingUsageHourlyDTO[]>([]);
  const [monthlyUsage, setMonthlyUsage] = React.useState<BillingUsageMonthlyDTO[]>([]);
  const [usageTotal, setUsageTotal] = React.useState(0);
  const [usagePage, setUsagePage] = React.useState(1);
  const [usagePageSize, setUsagePageSize] = React.useState(25);
  const [usageQuery, setUsageQuery] = React.useState("");
  const [usageBillingType, setUsageBillingType] = React.useState<BillingUsageType | "">("");
  const [usageSort, setUsageSort] = React.useState<BillingUsageSort>("newest");
  const [usageView, setUsageView] = React.useState<UsageTrendView>("daily");
  const [billingLoading, setBillingLoading] = React.useState(true);
  const [trendLoading, setTrendLoading] = React.useState(true);
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
  const [apiKeyDialogOpen, setAPIKeyDialogOpen] = React.useState(false);
  const [redemptionCode, setRedemptionCode] = React.useState("");
  const [redemptionLoading, setRedemptionLoading] = React.useState(false);
  const [pendingPayment, setPendingPayment] = React.useState<PendingPayment | null>(null);
  const [paymentVerificationLoading, setPaymentVerificationLoading] = React.useState(false);
  const trendRequestID = React.useRef(0);
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
      week: t("interval.week"),
      day: t("interval.day"),
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
      dailyCredit: (credit: string) => t("plans.features.dailyCredit", { credit }),
      weeklyCredit: (credit: string) => t("plans.features.weeklyCredit", { credit }),
      monthlyCredit: (credit: string) => t("plans.features.monthlyCredit", { credit }),
      freeModelsNotIncluded: t("plans.features.freeModelsNotIncluded"),
    }),
    [t],
  );
  const loadBillingData = React.useCallback(async () => {
    setBillingLoading(true);
    try {
      const [configData, overviewData] = await Promise.all([
        getBillingConfig(accessToken),
        getBillingOverview(accessToken),
      ]);
      setBillingConfig(configData.config);
      setBillingPlans(configData.config.plans);
      setBillingOverview(overviewData.overview);
    } catch (error) {
      toast.error(t("toasts.subscriptionLoadFailed"), { description: resolveErrorMessage(error, t("toasts.retryLater")) });
    } finally {
      setBillingLoading(false);
    }
  }, [accessToken, resolveErrorMessage, t]);

  React.useEffect(() => {
    void loadBillingData();
  }, [loadBillingData]);

  const loadUsageTrend = React.useCallback(async (view: UsageTrendView) => {
    const requestID = ++trendRequestID.current;
    setTrendLoading(true);
    try {
      if (view === "hourly") {
        const results = await listBillingHourlyUsage(accessToken);
        if (requestID === trendRequestID.current) setHourlyUsage(results);
      } else if (view === "monthly") {
        const results = await listBillingMonthlyUsage(accessToken, 12);
        if (requestID === trendRequestID.current) setMonthlyUsage(results);
      } else {
        const results = await listBillingDailyUsage(accessToken);
        if (requestID === trendRequestID.current) setDailyUsage(results);
      }
    } catch (error) {
      if (requestID === trendRequestID.current) {
        toast.error(t("toasts.subscriptionLoadFailed"), { description: resolveErrorMessage(error, t("toasts.retryLater")) });
      }
    } finally {
      if (requestID === trendRequestID.current) setTrendLoading(false);
    }
  }, [accessToken, resolveErrorMessage, t]);

  React.useEffect(() => {
    void loadUsageTrend(usageView);
  }, [loadUsageTrend, usageView]);

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

  const verifyPayment = React.useCallback(async (operationID: string) => {
    setPaymentVerificationLoading(true);
    try {
      const result = await verifyBillingOrder(accessToken, operationID);
      const status = result.order.status.trim().toLowerCase();
      if (!["completed", "paid", "success", "succeeded"].includes(status)) {
        toast.info(t("payment.statusPending"));
        return false;
      }
      await Promise.all([
        loadBillingData(),
        loadUsageLogs(1, 25, "", "", "newest"),
      ]);
      setPendingPayment(null);
      toast.success(t("toasts.paymentVerified"));
      return true;
    } catch (error) {
      toast.error(t("toasts.paymentVerifyFailed"), { description: resolveErrorMessage(error, t("toasts.retryLater")) });
      return false;
    } finally {
      setPaymentVerificationLoading(false);
    }
  }, [accessToken, loadBillingData, loadUsageLogs, resolveErrorMessage, t]);

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
      }, operationID);
      if (!data.checkout.checkoutURL && !data.checkout.qrCode) {
        toast.error(t("toasts.checkoutCreateFailed"), { description: t("toasts.checkoutURLMissing") });
        return;
      }
      setPendingPayment({ ...data.checkout, operationID });
      setPaymentDialogOpen(false);
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
      }, operationID);
      if (!data.checkout.checkoutURL && !data.checkout.qrCode) {
        toast.error(t("toasts.checkoutCreateFailed"), { description: t("toasts.checkoutURLMissing") });
        return;
      }
      setPendingPayment({ ...data.checkout, operationID });
      setTopUpDialogOpen(false);
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
                disabled={billingLoading || trendLoading || usageLoading}
                onClick={() => void Promise.all([
                  loadBillingData(),
                  loadUsageTrend(usageView),
                  loadUsageLogs(usagePage, usagePageSize, usageQuery, usageBillingType, usageSort),
                ])}
                aria-label={t("refresh")}
              >
                <RefreshCw className={billingLoading || trendLoading || usageLoading ? "animate-spin" : ""} />
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
        billingAccount={billingAccount}
        locale={locale}
        intervalLabels={intervalLabels}
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
        onOpenCreateKeyDialog={() => setAPIKeyDialogOpen(true)}
        onPricingDialogOpenChange={setPricingDialogOpen}
        onPaymentDialogOpenChange={setPaymentDialogOpen}
        onSelectPlan={(plan, price, isCurrent) => void handleSelectPlan(plan, price, isCurrent)}
        onPaymentProviderChange={setSelectedPaymentProvider}
        onConfirmPayment={() => void handleConfirmPayment()}
      />

      <section className="space-y-6 px-0.5 md:space-y-7 xl:space-y-8 xl:px-1">
        <Separator />
        <SubscriptionTrend
          hourlyUsage={hourlyUsage}
          dailyUsage={dailyUsage}
          monthlyUsage={monthlyUsage}
          loading={trendLoading}
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

      <SubscriptionAPIKeyDialog open={apiKeyDialogOpen} onOpenChange={setAPIKeyDialogOpen} />

      <PendingPaymentDialog
        open={pendingPayment !== null}
        onOpenChange={(open) => {
          if (!open && !paymentVerificationLoading) setPendingPayment(null);
        }}
        qrCode={pendingPayment?.qrCode ?? ""}
        checkoutURL={pendingPayment?.checkoutURL ?? ""}
        orderNo={pendingPayment?.orderNo ?? ""}
        payAmountCents={pendingPayment?.payAmountCents ?? 0}
        payCurrency={pendingPayment?.payCurrency ?? "CNY"}
        verifying={paymentVerificationLoading}
        onVerify={() => {
          if (pendingPayment) void verifyPayment(pendingPayment.operationID);
        }}
      />
    </SettingsPage>
  );
}
