"use client";

import * as React from "react";
import { Banknote, Check, KeyRound, Ticket, WalletCards } from "lucide-react";
import { useTranslations } from "next-intl";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { SpinnerLabel } from "@/components/ui/spinner";
import { resolvePlanDiscountPercent } from "@/features/settings/model/plan-discount";
import {
  formatAccountBalance,
  formatPlanCredit,
  formatPlanPrice,
  formatShortDate,
  isCurrentBillingPlan,
  planRank,
  resolveDefaultPrice,
  resolvePlanActionKind,
  resolvePlanActionLabel,
  resolvePlanButtonVariant,
  resolvePlanFeatures,
} from "@/features/settings/model/subscription-format";
import type { BillingOverviewData, BillingPlanDTO, BillingPlanPriceDTO } from "@/shared/api/billing.types";
import type { BillingDisplayOptions } from "@/shared/lib/billing-display";
import { formatBillingDisplayAmountFromUSD } from "@/shared/lib/billing-display";

type BillingMode = "period" | "usage" | "self";
type PaymentProvider = string;
type BillingAccount = NonNullable<BillingOverviewData["overview"]>["account"];

type SubscriptionIntervalLabels = {
  lifetime: string;
  year: string;
  month: string;
  week: string;
  day: string;
};

type PlanFeatureLabels = {
  dailyCredit: (credit: string) => string;
  weeklyCredit: (credit: string) => string;
  monthlyCredit: (credit: string) => string;
  freeModelsNotIncluded: string;
};

type PlanActionLabels = {
  current: string;
  unavailable: string;
  renew: string;
  subscribe: string;
  switch: string;
  upgrade: string;
  freeBlocked: string;
};

function paymentMethodLabel(provider: string): string {
  const normalized = provider.trim().toLowerCase();
  if (normalized === "alipay") return "Alipay";
  if (normalized === "wxpay") return "WeChat Pay";
  if (normalized === "stripe") return "Stripe";
  if (normalized === "airwallex") return "Airwallex";
  return provider;
}

function ActionRow({
  title,
  value,
  action,
}: {
  title: string;
  value?: string;
  action: React.ReactNode;
}) {
  return (
    <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between sm:gap-4">
      <div className="flex min-w-0 items-baseline gap-2">
        <p className="shrink-0 text-xs font-medium">{title}</p>
        {value ? <p className="max-w-[min(60vw,24rem)] truncate text-xs text-muted-foreground">{value}</p> : null}
      </div>
      <div className="self-start sm:self-auto sm:justify-self-end">{action}</div>
    </div>
  );
}

function ValueRow({
  title,
  value,
  action,
}: {
  title: string;
  value: string;
  action?: React.ReactNode;
}) {
  return (
    <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
      <p className="text-xs font-medium">{title}</p>
      <div className="flex min-w-0 max-w-full items-center gap-2 self-start rounded-lg bg-muted/35 px-2 py-1 text-xs text-muted-foreground sm:self-auto">
        <span className="max-w-[min(75vw,26rem)] truncate">{value}</span>
        {action}
      </div>
    </div>
  );
}

type SubscriptionSummaryProps = {
  billingMode: BillingMode;
  billingLoading: boolean;
  redemptionLoading: boolean;
  topUpLoading: boolean;
  paymentDisabled: boolean;
  billingPlans: BillingPlanDTO[];
  billingOverview: BillingOverviewData["overview"] | null;
  currentPlan: BillingPlanDTO | null;
  billingAccount: BillingAccount | null;
  locale: string;
  intervalLabels: SubscriptionIntervalLabels;
  planActionLabels: PlanActionLabels;
  planFeatureLabels: PlanFeatureLabels;
  paymentProviders: string[];
  selectedPlan: BillingPlanDTO | null;
  selectedPrice: BillingPlanPriceDTO | null;
  selectedPaymentProvider: PaymentProvider;
  checkoutPriceID: number | null;
  pricingDialogOpen: boolean;
  paymentDialogOpen: boolean;
  protectedPaidPlanRank: number;
  periodCredit: number;
  periodUsed: number;
  periodPercent: number;
  billingDisplay: BillingDisplayOptions;
  onOpenRedemptionDialog: () => void;
  onOpenTopUpDialog: () => void;
  onOpenCreateKeyDialog: () => void;
  onPricingDialogOpenChange: (open: boolean) => void;
  onPaymentDialogOpenChange: (open: boolean) => void;
  onSelectPlan: (plan: BillingPlanDTO, price: BillingPlanPriceDTO | null, isCurrent: boolean) => void;
  onPaymentProviderChange: (provider: PaymentProvider) => void;
  onConfirmPayment: () => void;
};

export function SubscriptionSummary({
  billingMode,
  billingLoading,
  redemptionLoading,
  topUpLoading,
  paymentDisabled,
  billingPlans,
  billingOverview,
  currentPlan,
  billingAccount,
  locale,
  intervalLabels,
  planActionLabels,
  planFeatureLabels,
  paymentProviders,
  selectedPlan,
  selectedPrice,
  selectedPaymentProvider,
  checkoutPriceID,
  pricingDialogOpen,
  paymentDialogOpen,
  protectedPaidPlanRank,
  periodCredit,
  periodUsed,
  periodPercent,
  billingDisplay,
  onOpenRedemptionDialog,
  onOpenTopUpDialog,
  onOpenCreateKeyDialog,
  onPricingDialogOpenChange,
  onPaymentDialogOpenChange,
  onSelectPlan,
  onPaymentProviderChange,
  onConfirmPayment,
}: SubscriptionSummaryProps) {
  const t = useTranslations("settings.subscriptionPage");
  const selectedPlanActionKind = selectedPlan
    ? resolvePlanActionKind(
      selectedPlan,
      selectedPrice,
      isCurrentBillingPlan(selectedPlan, currentPlan),
      currentPlan,
      protectedPaidPlanRank,
    )
    : "subscribe";
  const selectedRenewStartsAfterHigher = Boolean(
    selectedPlan
      && selectedPlanActionKind === "renew"
      && protectedPaidPlanRank > planRank(selectedPlan),
  );
  const paymentTitle = selectedPlanActionKind === "renew"
    ? t("payment.renewTitle")
    : selectedPlanActionKind === "upgrade"
      ? t("payment.upgradeTitle")
      : t("payment.title");
  const paymentImpactDescription = selectedPlanActionKind === "renew"
    ? selectedRenewStartsAfterHigher
      ? t("payment.renewAfterHigherDescription")
      : t("payment.renewDescription")
    : selectedPlanActionKind === "upgrade"
      ? t("payment.upgradeDescription")
      : null;
  const selectedPaymentAmountUSD = selectedPrice ? (selectedPrice.amountCents || 0) / 100 : 0;
  const paymentAmount = selectedPrice
    ? formatBillingDisplayAmountFromUSD(selectedPaymentAmountUSD, billingDisplay, { maximumFractionDigits: 2 })
    : "";

  return (
    <>
      {billingMode !== "self" ? (
        <section className="space-y-6 px-0.5 md:space-y-7 xl:space-y-8 xl:px-1">
          <ActionRow
            title={t("usageBilling.title")}
            value={t("usageBilling.balance", { value: formatAccountBalance(billingAccount?.balance ?? 0) })}
            action={
              <div className="grid grid-cols-2 gap-2 sm:grid-cols-4">
                <Button type="button" variant="outline" disabled={redemptionLoading} onClick={onOpenRedemptionDialog}>
                  <Ticket data-icon="inline-start" />
                  {t("redemption.open")}
                </Button>
                <Button type="button" variant="outline" disabled={topUpLoading || paymentDisabled} onClick={onOpenTopUpDialog}>
                  <Banknote data-icon="inline-start" />
                  {t("topUp.title")}
                </Button>
                <Button type="button" variant="outline" disabled={billingPlans.length === 0} onClick={() => onPricingDialogOpenChange(true)}>
                  <WalletCards data-icon="inline-start" />
                  {t("plans.actions.subscribe")}
                </Button>
                <Button type="button" variant="outline" onClick={onOpenCreateKeyDialog}>
                  <KeyRound data-icon="inline-start" />
                  {t("apiKey.open")}
                </Button>
              </div>
            }
          />

          {billingMode === "period" ? (
            <div className="space-y-3 rounded-md bg-muted/35 p-3 md:space-y-4">
                <div className="flex items-start justify-between gap-3 md:gap-4">
                  <div className="space-y-1">
                    <p className="text-xs font-medium">{t("periodUsage.title")}</p>
                    <p className="text-xs text-muted-foreground">
                      {billingOverview?.periodStartAt && billingOverview?.periodEndAt
                        ? `${formatShortDate(billingOverview.periodStartAt, locale)} - ${formatShortDate(billingOverview.periodEndAt, locale)}`
                        : t("periodUsage.currentPeriod")}
                    </p>
                  </div>
                  <p className="shrink-0 text-xs font-medium text-muted-foreground">{Math.round(periodPercent)}%</p>
                </div>
                <div className="space-y-2">
                  <div className="flex items-center justify-between gap-4 text-xs">
                    <span className="text-muted-foreground">{t("periodUsage.used", { value: formatPlanCredit(periodUsed, billingDisplay) })}</span>
                    <span className="text-muted-foreground">{t("periodUsage.total", { value: formatPlanCredit(periodCredit, billingDisplay) })}</span>
                  </div>
                  <div className="h-2 overflow-hidden rounded-full bg-muted">
                    <div className="h-full rounded-full bg-foreground/70" style={{ width: `${periodPercent}%` }} />
                  </div>
                </div>
            </div>
          ) : null}
        </section>
      ) : null}

      {billingMode === "self" ? (
        <section className="space-y-6 px-0.5 md:space-y-7 xl:space-y-8 xl:px-1">
          <ValueRow title={t("selfMode.title")} value={t("selfMode.value")} />
        </section>
      ) : null}

      <Dialog open={pricingDialogOpen} onOpenChange={onPricingDialogOpenChange}>
        <DialogContent className="sm:max-w-[760px] sm:p-5">
          <DialogHeader>
            <DialogTitle>{t("plans.title")}</DialogTitle>
          </DialogHeader>

          <div className="grid max-h-[min(72vh,42rem)] grid-cols-1 gap-2.5 overflow-y-auto pr-1 sm:grid-cols-2 md:grid-cols-3">
            {billingPlans.map((plan) => {
              const price = resolveDefaultPrice(plan);
              const isCurrent = isCurrentBillingPlan(plan, currentPlan);
              const actionKind = resolvePlanActionKind(plan, price, isCurrent, currentPlan, protectedPaidPlanRank);
              const actionLabel = resolvePlanActionLabel(actionKind, planActionLabels);
              const disabled = billingLoading || actionKind === "current" || actionKind === "freeBlocked" || actionKind === "unavailable" || checkoutPriceID === price?.id;
              const features = resolvePlanFeatures(plan, planFeatureLabels, billingDisplay).slice(0, 2);
              const isSelected = selectedPlan?.id === plan.id;
              const isHighlighted = isCurrent || isSelected;
              const buttonVariant = resolvePlanButtonVariant(actionKind);
              const discountPercent = price ? resolvePlanDiscountPercent(plan.originalPriceCents, price.amountCents) : null;
              const originalPrice = price && discountPercent
                ? formatPlanPrice({ ...price, amountCents: plan.originalPriceCents }, intervalLabels, billingDisplay, plan.validityDays)
                : "";
              return (
                <article
                  key={plan.id}
                  className={[
                    "flex min-h-52 flex-col rounded-lg border bg-background p-3 transition-colors",
                    isHighlighted ? "border-foreground ring-1 ring-foreground" : "border-border/70 hover:bg-muted/25",
                  ].join(" ")}
                >
                  <div className="flex min-w-0 items-start justify-between gap-2">
                    <h3 className="truncate text-sm font-semibold">{plan.name}</h3>
                    {isCurrent ? <Badge variant="secondary">{t("plans.currentBadge")}</Badge> : null}
                  </div>

                  <p className="mt-3 text-base font-semibold">{formatPlanPrice(price, intervalLabels, billingDisplay, plan.validityDays)}</p>
                  <div className="mt-1 flex min-h-5 flex-wrap items-center gap-2 text-xs text-muted-foreground">
                    {discountPercent ? (
                      <>
                        <span className="line-through">{t("plans.originalPrice", { price: originalPrice })}</span>
                        <Badge variant="outline">{t("plans.discount", { percent: discountPercent })}</Badge>
                      </>
                    ) : null}
                  </div>

                  <div className="mt-3 grid gap-1.5">
                    {features.map((feature) => (
                      <div key={feature} className="flex items-start gap-2 text-xs text-muted-foreground">
                        <Check className="mt-0.5 size-3 shrink-0 text-foreground" />
                        <span className="line-clamp-2 leading-4">{feature}</span>
                      </div>
                    ))}
                  </div>

                  <Button
                    type="button"
                    size="sm"
                    className="mt-auto w-full shadow-none"
                    variant={buttonVariant}
                    disabled={disabled}
                    onClick={() => onSelectPlan(plan, price, isCurrent)}
                  >
                    {checkoutPriceID === price?.id ? <SpinnerLabel>{t("actions.processing")}</SpinnerLabel> : actionLabel}
                  </Button>
                </article>
              );
            })}
          </div>
        </DialogContent>
      </Dialog>

      <Dialog open={paymentDialogOpen} onOpenChange={onPaymentDialogOpenChange}>
        <DialogContent className="sm:max-w-[420px]">
          <DialogHeader>
            <DialogTitle>{paymentTitle}</DialogTitle>
            <DialogDescription>
              <span className="block">
                {selectedPlan && selectedPrice
                  ? `${selectedPlan.name} · ${formatPlanPrice(selectedPrice, intervalLabels, billingDisplay, selectedPlan.validityDays)}`
                  : t("payment.description")}
              </span>
              {paymentImpactDescription ? <span className="mt-1 block">{paymentImpactDescription}</span> : null}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            {paymentProviders.map((provider) => (
              <button
                key={provider}
                type="button"
                className={`flex w-full items-center justify-between rounded-lg border px-3 py-3 text-left ${
                  selectedPaymentProvider === provider ? "border-foreground bg-muted/25" : "border-border bg-transparent"
                }`}
                disabled={paymentDisabled}
                onClick={() => onPaymentProviderChange(provider)}
              >
                <span className="space-y-1">
                  <span className="block text-xs font-medium">{paymentMethodLabel(provider)}</span>
                  <span className="block text-xs text-muted-foreground">{paymentAmount || t("payment.card")}</span>
                </span>
                {selectedPaymentProvider === provider ? <Check className="size-4" /> : null}
              </button>
            ))}
          </div>
          <DialogFooter>
            <Button type="button" variant="ghost" onClick={() => onPaymentDialogOpenChange(false)} disabled={checkoutPriceID === selectedPrice?.id}>
              {t("actions.cancel")}
            </Button>
            <Button type="button" disabled={paymentDisabled || !selectedPrice || checkoutPriceID === selectedPrice.id} onClick={onConfirmPayment}>
              {checkoutPriceID === selectedPrice?.id ? <SpinnerLabel>{t("actions.processing")}</SpinnerLabel> : t("payment.continue")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
