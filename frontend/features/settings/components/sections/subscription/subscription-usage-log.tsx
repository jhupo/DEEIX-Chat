"use client";

import * as React from "react";
import { useTranslations } from "next-intl";

import { Table, TableBody, TableCell, TableEmptyRow, TableHead, TableHeader, TableLoadingRow, TableRow } from "@/components/ui/table";
import { TablePagination, TableToolbar } from "@/components/ui/table-tools";
import { useVirtualTableRows, VirtualTablePaddingRow } from "@/components/ui/virtual-table";
import { useAppLocale } from "@/i18n/app-i18n-provider";
import type { BillingUsageLedgerDTO, BillingUsageSort, BillingUsageType } from "@/shared/api/billing.types";
import type { BillingDisplayOptions } from "@/shared/lib/billing-display";
import {
  formatFormulaTokenCount,
  formatLatency,
  formatUsageCost,
  formatUsageLogTime,
} from "@/features/settings/model/subscription-format";

export function SubscriptionUsageLog({
  items,
  total,
  loading,
  page,
  pageSize,
  query,
  billingType,
  sort,
  billingDisplay,
  onQueryChange,
  onBillingTypeChange,
  onSortChange,
  onRefresh,
  onPageChange,
  onPageSizeChange,
}: {
  items: BillingUsageLedgerDTO[];
  total: number;
  loading: boolean;
  page: number;
  pageSize: number;
  query: string;
  billingType: BillingUsageType | "";
  sort: BillingUsageSort;
  billingDisplay: BillingDisplayOptions;
  onQueryChange: (value: string) => void;
  onBillingTypeChange: (value: BillingUsageType | "") => void;
  onSortChange: (value: BillingUsageSort) => void;
  onRefresh: () => void;
  onPageChange: (page: number) => void;
  onPageSizeChange: (pageSize: number) => void;
}) {
  const t = useTranslations("settings.subscriptionPage.usageLog");
  const { locale } = useAppLocale();
  const billingTypeOptions = React.useMemo(
    () => [
      { label: t("filters.all"), value: "" },
      { label: t("filters.balance"), value: "balance" },
      { label: t("filters.subscription"), value: "subscription" },
    ],
    [t],
  );
  const sortOptions = React.useMemo(
    () => [
      { label: t("sort.newest"), value: "newest" },
      { label: t("sort.oldest"), value: "oldest" },
    ],
    [t],
  );
  const virtualRows = useVirtualTableRows(items, {
    enabled: items.length > 100,
    estimateSize: 40,
  });
  const initialLoading = loading && items.length === 0;
  const showRows = items.length > 0;
  const pageCount = Math.max(1, Math.ceil(total / pageSize));

  return (
    <div className="space-y-3">
      <div className="flex h-9 items-center">
        <h3 className="text-sm font-semibold">{t("title")}</h3>
      </div>

      <TableToolbar
        query={query}
        onQueryChange={onQueryChange}
        queryPlaceholder={t("searchModel")}
        filters={[{
          key: "billingType",
          label: t("type"),
          value: billingType,
          onValueChange: onBillingTypeChange,
          options: billingTypeOptions,
        }]}
        sort={{ value: sort, onValueChange: onSortChange, options: sortOptions }}
        loading={loading}
        onRefresh={onRefresh}
      />

      <Table
        className="table-fixed"
        viewportRef={virtualRows.viewportRef}
        viewportClassName={virtualRows.viewportClassName}
        viewportStyle={virtualRows.viewportStyle}
      >
        <TableHeader>
          <TableRow>
            <TableHead className="w-[10rem]">{t("columns.time")}</TableHead>
            <TableHead className="w-[10rem]">{t("columns.model")}</TableHead>
            <TableHead className="w-[6rem] text-right">{t("columns.inputTokens")}</TableHead>
            <TableHead className="w-[6rem] text-right">{t("columns.outputTokens")}</TableHead>
            <TableHead className="w-[6rem] text-right">{t("columns.totalTokens")}</TableHead>
            <TableHead className="w-[7rem] text-right">{t("columns.cost")}</TableHead>
            <TableHead className="w-[6rem] text-right">{t("columns.duration")}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {initialLoading ? <TableLoadingRow colSpan={7} /> : null}
          {!loading && items.length === 0 ? <TableEmptyRow colSpan={7}>{t("empty")}</TableEmptyRow> : null}
          {showRows ? <VirtualTablePaddingRow colSpan={7} height={virtualRows.paddingTop} /> : null}
          {showRows
            ? virtualRows.rows.map(({ item }) => (
                <TableRow key={item.id}>
                  <TableCell className="text-xs text-muted-foreground">{formatUsageLogTime(item.createdAt, locale)}</TableCell>
                  <TableCell className="w-[10rem] max-w-[10rem] text-xs font-medium">
                    <div className="truncate" title={item.model}>{item.model || "-"}</div>
                  </TableCell>
                  <TableCell className="text-right text-xs tabular-nums">{formatFormulaTokenCount(item.inputTokens)}</TableCell>
                  <TableCell className="text-right text-xs tabular-nums">{formatFormulaTokenCount(item.outputTokens)}</TableCell>
                  <TableCell className="text-right text-xs font-medium tabular-nums">{formatFormulaTokenCount(item.totalTokens)}</TableCell>
                  <TableCell className="text-right text-xs font-medium tabular-nums">
                    {formatUsageCost(Number(item.actualCost), billingDisplay)}
                  </TableCell>
                  <TableCell className="text-right text-xs text-muted-foreground">{formatLatency(item.durationMS)}</TableCell>
                </TableRow>
              ))
            : null}
          {showRows ? <VirtualTablePaddingRow colSpan={7} height={virtualRows.paddingBottom} /> : null}
        </TableBody>
      </Table>

      <TablePagination
        total={total}
        page={page}
        pageCount={pageCount}
        pageSize={pageSize}
        onPageChange={onPageChange}
        onPageSizeChange={onPageSizeChange}
        loading={loading}
      />
    </div>
  );
}
