"use client";

import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { SpinnerLabel } from "@/components/ui/spinner";
import { useDialogSnapshot } from "@/shared/hooks/use-dialog-snapshot";
import {
  billingDisplayAmountToUSD,
  billingDisplayInputSymbol,
} from "@/features/settings/model/subscription-format";
import type { BillingDisplayOptions } from "@/shared/lib/billing-display";
import { formatBillingDisplayAmountFromUSD } from "@/shared/lib/billing-display";
import { calculateTopUpPreview } from "./top-up-preview";

type PaymentProvider = string;

function paymentMethodLabel(provider: string): string {
  const normalized = provider.trim().toLowerCase();
  if (normalized === "alipay") return "Alipay";
  if (normalized === "wxpay") return "WeChat Pay";
  if (normalized === "stripe") return "Stripe";
  if (normalized === "airwallex") return "Airwallex";
  return provider;
}

type TopUpDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  amount: string;
  currentBalance: string;
  billingLoading: boolean;
  topUpLoading: boolean;
  paymentDisabled: boolean;
  paymentProviders: string[];
  paymentMethods: { id: string; currency: string }[];
  selectedPaymentProvider: PaymentProvider;
  billingDisplay: BillingDisplayOptions;
  balanceRechargeMultiplier: number;
  rechargeFeeRate: number;
  onAmountChange: (value: string) => void;
  onPaymentProviderChange: (provider: PaymentProvider) => void;
  onSubmit: () => void;
};

export function TopUpDialog({
  open,
  onOpenChange,
  amount,
  currentBalance,
  billingLoading,
  topUpLoading,
  paymentDisabled,
  paymentProviders,
  paymentMethods,
  selectedPaymentProvider,
  billingDisplay,
  balanceRechargeMultiplier,
  rechargeFeeRate,
  onAmountChange,
  onPaymentProviderChange,
  onSubmit,
}: TopUpDialogProps) {
  const t = useTranslations("settings.subscriptionPage");
  const displayAmount = Number(amount);
  const paymentAmountUSD = billingDisplayAmountToUSD(displayAmount, billingDisplay);
  const selectedMethod = paymentMethods.find((method) => method.id === selectedPaymentProvider);
  const paymentCurrency = selectedMethod?.currency.trim().toUpperCase() || "USD";
  const preview = calculateTopUpPreview(paymentAmountUSD, paymentCurrency, billingDisplay.usdToCnyRate, balanceRechargeMultiplier, rechargeFeeRate);
  const paymentAmount = new Intl.NumberFormat(undefined, { style: "currency", currency: paymentCurrency }).format(preview.paymentAmount);
  const creditedAmount = formatBillingDisplayAmountFromUSD(preview.creditedUSD, billingDisplay, { maximumFractionDigits: 2 });
  const inputSymbol = billingDisplayInputSymbol(billingDisplay);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[420px]">
        <DialogHeader>
          <DialogTitle>{t("topUp.title")}</DialogTitle>
          <DialogDescription>{t("topUp.description")}</DialogDescription>
        </DialogHeader>

        <div className="space-y-1.5">
          <div className="flex items-center justify-between gap-3">
            <p className="text-xs text-muted-foreground">{t("topUp.amount")}</p>
            <p className="truncate text-xs text-muted-foreground tabular-nums">
              {t("topUp.currentBalance", { value: currentBalance })}
            </p>
          </div>
          <div className="relative">
            <span className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-xs text-muted-foreground">{inputSymbol}</span>
            <Input
              value={amount}
              type="number"
              min="0"
              step="0.01"
              className="pl-7"
              onChange={(event) => onAmountChange(event.target.value)}
              disabled={billingLoading || topUpLoading || paymentDisabled}
              aria-label={t("topUp.amountAria")}
            />
          </div>
        </div>

        {!paymentDisabled ? (
          <div className="space-y-2">
            <p className="text-xs text-muted-foreground">{t("payment.method")}</p>
            <div className="grid grid-cols-2 gap-2">
              {paymentProviders.map((provider) => (
                <button
                  key={provider}
                  type="button"
                  className={`flex min-h-9 flex-col items-center justify-center rounded-md border px-2 py-1 text-xs ${
                    selectedPaymentProvider === provider ? "border-foreground bg-muted/25 font-medium" : "border-border bg-transparent text-muted-foreground"
                  }`}
                  disabled={billingLoading || topUpLoading || paymentDisabled}
                  onClick={() => onPaymentProviderChange(provider)}
                >
                  <span>{paymentMethodLabel(provider)}</span>
                  <span className="text-[11px] font-normal tabular-nums opacity-80">{provider === selectedPaymentProvider ? paymentAmount : ""}</span>
                </button>
              ))}
            </div>
          </div>
        ) : null}

        <div className="grid grid-cols-2 gap-3 border-y py-3 text-xs">
          <div>
            <p className="text-muted-foreground">{t("topUp.paymentAmount")}</p>
            <p className="mt-1 font-medium tabular-nums">{paymentAmount}</p>
          </div>
          <div>
            <p className="text-muted-foreground">{t("topUp.creditedBalance")}</p>
            <p className="mt-1 font-medium tabular-nums">{creditedAmount}</p>
          </div>
        </div>

        <DialogFooter>
          <Button type="button" variant="ghost" onClick={() => onOpenChange(false)} disabled={topUpLoading}>
            {t("actions.cancel")}
          </Button>
          <Button type="button" disabled={billingLoading || topUpLoading || paymentDisabled} onClick={onSubmit}>
            {topUpLoading ? <SpinnerLabel>{t("actions.processing")}</SpinnerLabel> : t("topUp.confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

type RedemptionDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  code: string;
  billingLoading: boolean;
  redemptionLoading: boolean;
  onCodeChange: (value: string) => void;
  onSubmit: () => void;
};

export function RedemptionDialog({
  open,
  onOpenChange,
  code,
  billingLoading,
  redemptionLoading,
  onCodeChange,
  onSubmit,
}: RedemptionDialogProps) {
  const t = useTranslations("settings.subscriptionPage");
  const stableCode = useDialogSnapshot(open ? code : null) ?? "";

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[420px]">
        <DialogHeader>
          <DialogTitle>{t("redemption.title")}</DialogTitle>
          <DialogDescription>{t("redemption.description")}</DialogDescription>
        </DialogHeader>
        <div className="space-y-1">
          <p className="text-xs text-muted-foreground">{t("redemption.code")}</p>
          <Input
            value={stableCode}
            autoComplete="off"
            className="font-mono"
            disabled={billingLoading || redemptionLoading}
            onChange={(event) => onCodeChange(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === "Enter") {
                event.preventDefault();
                onSubmit();
              }
            }}
            aria-label={t("redemption.code")}
          />
        </div>
        <DialogFooter>
          <Button type="button" variant="ghost" onClick={() => onOpenChange(false)} disabled={redemptionLoading}>
            {t("actions.cancel")}
          </Button>
          <Button type="button" disabled={billingLoading || redemptionLoading} onClick={onSubmit}>
            {redemptionLoading ? <SpinnerLabel>{t("actions.processing")}</SpinnerLabel> : t("redemption.confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
