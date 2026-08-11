"use client";

import * as React from "react";
import { Activity, BadgeDollarSign, Braces } from "lucide-react";
import { Bar, BarChart, CartesianGrid, XAxis, YAxis } from "recharts";
import { useTranslations } from "next-intl";

import { ChartContainer, ChartTooltip } from "@/components/ui/chart";
import type { ChartConfig } from "@/components/ui/chart";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useAppLocale } from "@/i18n/app-i18n-provider";
import type { BillingUsageDailyDTO, BillingUsageMonthlyDTO } from "@/shared/api/billing.types";
import {
  formatDay,
  formatFormulaTokenCount,
  formatFullMonthLabel,
  formatMonthLabel,
  formatShortDate,
  formatTokenCount,
  formatUsageAxisTokens,
  formatUsageSummaryCost,
} from "@/features/settings/model/subscription-format";
import type { BillingDisplayOptions } from "@/shared/lib/billing-display";

type UsagePoint = {
  key: string;
  label: string;
  fullLabel: string;
  actualCost: string;
  totalTokens: number;
  callCount: number;
};

type UsageTrendStats = {
  totalCost: number;
  totalTokens: number;
  totalCalls: number;
};

export type UsageTrendView = "daily" | "monthly";

const chartConfig = {
  totalTokens: {
    label: "Tokens",
    color: "var(--chart-1)",
  },
} satisfies ChartConfig;

function MetricTile({ label, value, icon }: { label: string; value: string; icon: React.ReactNode }) {
  return (
    <div className="min-w-0 rounded-md bg-muted/35 px-3 py-3.5 md:px-4 md:py-4">
      <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
        <span className="text-foreground/55">{icon}</span>
        <span>{label}</span>
      </div>
      <p className="mt-2 truncate text-base font-semibold tabular-nums text-foreground md:text-lg">{value}</p>
    </div>
  );
}

function UsageMetrics({ stats, billingDisplay }: { stats: UsageTrendStats; billingDisplay: BillingDisplayOptions }) {
  const t = useTranslations("settings.subscriptionPage.usageTrend.metrics");
  return (
    <div className="grid grid-cols-1 gap-2 text-xs sm:grid-cols-3">
      <MetricTile label={t("totalCost")} value={formatUsageSummaryCost(stats.totalCost, billingDisplay)} icon={<BadgeDollarSign className="size-4" />} />
      <MetricTile label={t("totalTokens")} value={formatFormulaTokenCount(stats.totalTokens)} icon={<Braces className="size-4" />} />
      <MetricTile label={t("totalCalls")} value={stats.totalCalls.toLocaleString("en-US")} icon={<Activity className="size-4" />} />
    </div>
  );
}

function calculateStats(items: UsagePoint[]): UsageTrendStats {
  return items.reduce(
    (total, item) => ({
      totalCost: total.totalCost + Number(item.actualCost),
      totalTokens: total.totalTokens + item.totalTokens,
      totalCalls: total.totalCalls + item.callCount,
    }),
    { totalCost: 0, totalTokens: 0, totalCalls: 0 },
  );
}

function UsageTooltip({ active, payload, billingDisplay }: {
  active?: boolean;
  payload?: Array<{ payload?: UsagePoint }>;
  billingDisplay: BillingDisplayOptions;
}) {
  const t = useTranslations("settings.subscriptionPage.usageTrend.tooltip");
  const item = payload?.[0]?.payload;
  if (!active || !item) return null;
  return (
    <div className="grid min-w-[9rem] gap-1.5 rounded-md border border-border/50 bg-background px-2.5 py-2 text-xs shadow-md">
      <p className="font-medium">{item.fullLabel}</p>
      <div className="grid gap-1 text-muted-foreground">
        <div className="flex items-center justify-between gap-6">
          <span>{t("tokens")}</span>
          <span className="font-medium text-foreground tabular-nums">{formatTokenCount(item.totalTokens)}</span>
        </div>
        <div className="flex items-center justify-between gap-6">
          <span>{t("cost")}</span>
          <span className="font-medium text-foreground tabular-nums">{formatUsageSummaryCost(Number(item.actualCost), billingDisplay)}</span>
        </div>
        <div className="flex items-center justify-between gap-6">
          <span>{t("calls")}</span>
          <span className="font-medium text-foreground tabular-nums">{item.callCount.toLocaleString("en-US")}</span>
        </div>
      </div>
    </div>
  );
}

function UsageChart({ title, items, loading, billingDisplay }: {
  title: string;
  items: UsagePoint[];
  loading: boolean;
  billingDisplay: BillingDisplayOptions;
}) {
  const t = useTranslations("settings.subscriptionPage.usageTrend");
  const rangeLabel = items.length > 0 ? `${items[0]?.fullLabel} - ${items[items.length - 1]?.fullLabel}` : "";
  const hasUsageData = items.some((item) => Number(item.actualCost) > 0 || item.totalTokens > 0 || item.callCount > 0);
  return (
    <div className="space-y-3 rounded-md bg-muted/35 p-3">
      <div className="flex h-7 items-center justify-between gap-3 px-1">
        <p className="text-xs font-medium text-foreground">{title}</p>
        {rangeLabel ? <p className="truncate text-xs text-muted-foreground">{rangeLabel}</p> : null}
      </div>
      {loading ? <UsageChartSkeleton /> : null}
      {!loading && !hasUsageData ? <div className="flex h-[220px] items-center justify-center text-xs text-muted-foreground">{t("empty")}</div> : null}
      {!loading && hasUsageData ? (
        <ChartContainer config={chartConfig} className="h-[260px] w-full aspect-auto">
          <BarChart data={items} margin={{ top: 8, right: 8, left: 8, bottom: 0 }}>
            <CartesianGrid vertical={false} strokeDasharray="3 3" />
            <XAxis dataKey="label" tickLine={false} axisLine={false} tickMargin={8} minTickGap={24} interval="equidistantPreserveStart" />
            <YAxis width={64} tickLine={false} axisLine={false} tickMargin={6} tickFormatter={(value: number) => formatUsageAxisTokens(value)} />
            <ChartTooltip cursor={false} content={<UsageTooltip billingDisplay={billingDisplay} />} />
            <Bar dataKey="totalTokens" fill="var(--color-totalTokens)" radius={[4, 4, 2, 2]} maxBarSize={42} isAnimationActive animationDuration={240} animationEasing="ease-out" />
          </BarChart>
        </ChartContainer>
      ) : null}
    </div>
  );
}

function UsageChartSkeleton() {
  return (
    <div className="flex h-[260px] items-end gap-2 px-2 pb-8 pt-8">
      {Array.from({ length: 12 }).map((_, index) => (
        <Skeleton key={`usage-chart-skeleton-${index}`} className="flex-1 rounded-t-sm" style={{ height: `${28 + ((index * 17) % 58)}%` }} />
      ))}
    </div>
  );
}

export function SubscriptionTrend({
  dailyUsage,
  monthlyUsage,
  loading,
  view,
  billingDisplay,
  onViewChange,
}: {
  dailyUsage: BillingUsageDailyDTO[];
  monthlyUsage: BillingUsageMonthlyDTO[];
  loading: boolean;
  view: UsageTrendView;
  billingDisplay: BillingDisplayOptions;
  onViewChange: (view: UsageTrendView) => void;
}) {
  const t = useTranslations("settings.subscriptionPage");
  const { locale } = useAppLocale();
  const dailyPoints = React.useMemo<UsagePoint[]>(
    () => [...dailyUsage]
      .sort((left, right) => left.usageDate.localeCompare(right.usageDate))
      .map((item) => ({
        key: item.usageDate,
        label: formatDay(item.usageDate),
        fullLabel: formatShortDate(item.usageDate, locale),
        actualCost: item.actualCost,
        totalTokens: item.totalTokens,
        callCount: item.callCount,
      })),
    [dailyUsage, locale],
  );
  const monthlyPoints = React.useMemo<UsagePoint[]>(
    () => [...monthlyUsage]
      .sort((left, right) => left.monthStartAt.localeCompare(right.monthStartAt))
      .map((item) => ({
        key: item.monthStartAt,
        label: formatMonthLabel(item.monthStartAt, locale),
        fullLabel: formatFullMonthLabel(item.monthStartAt, locale),
        actualCost: item.actualCost,
        totalTokens: item.totalTokens,
        callCount: item.callCount,
      })),
    [locale, monthlyUsage],
  );
  const points = view === "daily" ? dailyPoints : monthlyPoints;
  const stats = React.useMemo(() => calculateStats(points), [points]);

  return (
    <div className="space-y-4 md:space-y-5">
      <div className="flex h-9 items-center justify-between gap-3">
        <h3 className="text-sm font-semibold">{view === "daily" ? t("usageTrend.dailyTitle") : t("usageTrend.monthlyTitle")}</h3>
        <Tabs value={view} onValueChange={(value) => onViewChange(value as UsageTrendView)}>
          <TabsList>
            <TabsTrigger value="daily">{t("usageTrend.daily")}</TabsTrigger>
            <TabsTrigger value="monthly">{t("usageTrend.monthly")}</TabsTrigger>
          </TabsList>
        </Tabs>
      </div>
      <UsageMetrics stats={stats} billingDisplay={billingDisplay} />
      <UsageChart
        title={view === "daily" ? t("usageTrend.dailyUsage") : t("usageTrend.monthlyUsage")}
        items={points}
        loading={loading}
        billingDisplay={billingDisplay}
      />
    </div>
  );
}
