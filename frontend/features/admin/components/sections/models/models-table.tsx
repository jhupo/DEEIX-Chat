"use client";

import * as React from "react";
import { CircleOff, MoreHorizontal, Pencil, RotateCcw, Trash2 } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import {
  Table,
  TableBody,
  TableCell,
  TableEmptyRow,
  TableHead,
  TableHeader,
  TableLoadingRow,
  TableRow,
} from "@/components/ui/table";
import { useVirtualTableRows, VirtualTablePaddingRow } from "@/components/ui/virtual-table";
import type {
  AdminLLMModelAccessScope,
  AdminLLMModelDTO,
  AdminLLMStatus,
} from "@/features/admin/api/llm.types";
import { ADAPTER_LABELS, formatDateTime } from "@/features/admin/types/llm";
import { sortProtocolsForDisplay } from "@/features/admin/utils/llm-display";
import { ModelIcon } from "@/shared/components/model-icon";
import { resolveModelIconURL, resolveModelIdentity } from "@/shared/lib/model-identity";
import { parseProtocolsJSON } from "@/shared/lib/model-protocols";
import { parseKindsJSON } from "@/shared/model/llm-schema";

function ProtocolBadges({ protocols }: { protocols: string[] }) {
  const sortedProtocols = sortProtocolsForDisplay(protocols);
  if (sortedProtocols.length === 0) {
    return <span className="text-muted-foreground">-</span>;
  }
  return (
    <div className="flex min-w-0 flex-nowrap items-center gap-1">
      {sortedProtocols.map((protocol) => (
        <Badge key={protocol} variant="secondary" className="whitespace-nowrap">
          {ADAPTER_LABELS[protocol] ?? protocol}
        </Badge>
      ))}
    </div>
  );
}

function KindsBadges({ kindsJSON }: { kindsJSON: string | null | undefined }) {
  const t = useTranslations("adminModels");
  const kinds = parseKindsJSON(kindsJSON);
  if (kinds.length === 0) {
    return <span className="text-muted-foreground">-</span>;
  }
  return (
    <div className="flex min-w-0 flex-nowrap items-center justify-start gap-1 overflow-hidden">
      {kinds.map((kind) => (
        <Badge key={kind} variant="secondary">
          {["chat", "audio", "image_gen", "image_edit", "video_gen"].includes(kind)
            ? t(`kinds.${kind}`)
            : kind}
        </Badge>
      ))}
    </div>
  );
}

type ModelsTableProps = {
  items: AdminLLMModelDTO[];
  loading: boolean;
  selectedModelIDs: Set<number>;
  onSelectedModelIDsChange: React.Dispatch<React.SetStateAction<Set<number>>>;
  onEdit: (item: AdminLLMModelDTO) => void;
  onToggleStatus: (item: AdminLLMModelDTO, status: AdminLLMStatus) => void;
  onToggleAccessScope: (item: AdminLLMModelDTO, scope: AdminLLMModelAccessScope) => void;
  onDelete: (item: AdminLLMModelDTO) => void;
};

export function ModelsTable({
  items,
  loading,
  selectedModelIDs,
  onSelectedModelIDsChange,
  onEdit,
  onToggleStatus,
  onToggleAccessScope,
  onDelete,
}: ModelsTableProps) {
  const t = useTranslations("adminModels");
  const locale = useLocale();
  const virtualRows = useVirtualTableRows(items, {
    enabled: items.length > 100,
    estimateSize: 40,
  });
  const visibleIDs = React.useMemo(() => items.map((item) => item.id), [items]);
  const selectedVisibleCount = visibleIDs.filter((id) => selectedModelIDs.has(id)).length;
  const allModelsSelected = visibleIDs.length > 0 && selectedVisibleCount === visibleIDs.length;
  const someModelsSelected = selectedVisibleCount > 0 && !allModelsSelected;
  const initialLoading = loading && items.length === 0;
  const showRows = items.length > 0;

  function selectModel(modelID: number, selected: boolean) {
    onSelectedModelIDsChange((current) => {
      const next = new Set(current);
      if (selected) {
        next.add(modelID);
      } else {
        next.delete(modelID);
      }
      return next;
    });
  }

  function selectAllModels(selected: boolean) {
    onSelectedModelIDsChange((current) => {
      const next = new Set(current);
      visibleIDs.forEach((id) => {
        if (selected) {
          next.add(id);
        } else {
          next.delete(id);
        }
      });
      return next;
    });
  }

  return (
    <Table
      viewportRef={virtualRows.viewportRef}
      viewportClassName={virtualRows.viewportClassName}
      viewportStyle={{
        ...virtualRows.viewportStyle,
        height: "clamp(18rem, calc(100svh - 18rem), 40rem)",
      }}
    >
      <TableHeader>
        <TableRow className="hover:bg-transparent">
          <TableHead className="w-[44px] py-1.5 text-center">
            <div className="flex h-7 items-center justify-center">
              <Checkbox
                checked={allModelsSelected ? true : someModelsSelected ? "indeterminate" : false}
                onCheckedChange={(checked) => selectAllModels(checked === true)}
                aria-label={t("table.selectAllModels")}
              />
            </div>
          </TableHead>
          <TableHead>{t("platformModel")}</TableHead>
          <TableHead>{t("table.kind")}</TableHead>
          <TableHead>{t("fields.protocol")}</TableHead>
          <TableHead className="w-[120px]">{t("table.vendor")}</TableHead>
          <TableHead className="w-[72px] text-center">{t("fields.status")}</TableHead>
          <TableHead className="w-[112px]">{t("table.accessScope")}</TableHead>
          <TableHead className="w-[140px]">{t("sheet.updatedAt")}</TableHead>
          <TableHead className="w-[56px]" stickyEnd />
        </TableRow>
      </TableHeader>

      <TableBody>
        {initialLoading ? <TableLoadingRow colSpan={9} /> : null}
        {items.length === 0 && !loading ? <TableEmptyRow colSpan={9}>{t("table.empty")}</TableEmptyRow> : null}
        {showRows ? <VirtualTablePaddingRow colSpan={9} height={virtualRows.paddingTop} /> : null}
        {showRows
          ? virtualRows.rows.map(({ item }) => {
              const identity = resolveModelIdentity({
                code: item.platformModelName,
                vendor: item.vendor,
                icon: item.icon,
              });
              const iconURL = resolveModelIconURL(identity.modelIcon);
              const vendorLabel = item.vendorName.trim() || item.vendor.trim();
              const vendorIconURL = resolveModelIconURL(item.vendorIcon);
              const showVendor = item.vendor.trim().toLowerCase() !== "unknown" && vendorLabel;
              const inactive = item.status !== "active";
              const selected = selectedModelIDs.has(item.id);

              return (
                <TableRow key={item.id} tone={inactive ? "muted" : undefined} selected={selected}>
                  <TableCell className="w-[44px] whitespace-nowrap py-1.5">
                    <div className="flex h-7 items-center justify-center">
                      <Checkbox
                        checked={selected}
                        onCheckedChange={(checked) => selectModel(item.id, checked === true)}
                        aria-label={t("table.selectModel", { name: item.platformModelName })}
                      />
                    </div>
                  </TableCell>
                  <TableCell className="py-1.5">
                    <div className="flex min-w-0 items-center gap-2">
                      <ModelIcon iconUrl={iconURL} label={item.platformModelName} />
                      <span className="min-w-0 flex-1 truncate text-xs font-medium leading-5">
                        {item.platformModelName.trim()}
                      </span>
                    </div>
                  </TableCell>
                  <TableCell className="py-1.5">
                    <KindsBadges kindsJSON={item.kindsJSON} />
                  </TableCell>
                  <TableCell className="py-1.5">
                    <ProtocolBadges protocols={parseProtocolsJSON(item.protocolsJSON)} />
                  </TableCell>
                  <TableCell className="w-[120px] py-1.5">
                    {showVendor ? (
                      <div className="flex min-w-0 items-center gap-1.5">
                        {vendorIconURL ? <ModelIcon iconUrl={vendorIconURL} label={vendorLabel} size={14} /> : null}
                        <span className="block max-w-[92px] truncate text-xs text-muted-foreground">
                          {vendorLabel}
                        </span>
                      </div>
                    ) : (
                      <span className="text-xs text-muted-foreground">-</span>
                    )}
                  </TableCell>
                  <TableCell className="w-[72px] whitespace-nowrap py-1.5">
                    <div className="flex h-7 items-center justify-center">
                      <Switch
                        size="sm"
                        checked={item.status === "active"}
                        onCheckedChange={(checked) => onToggleStatus(item, checked ? "active" : "inactive")}
                        aria-label={t("table.modelStatusAria", { name: item.platformModelName })}
                      />
                    </div>
                  </TableCell>
                  <TableCell className="w-[112px] whitespace-nowrap py-1.5">
                    <div className="flex h-7 items-center">
                      <Select
                        value={item.accessScope === "internal" ? "internal" : "public"}
                        onValueChange={(value) => onToggleAccessScope(item, value as AdminLLMModelAccessScope)}
                      >
                        <SelectTrigger
                          size="sm"
                          className="h-7 w-[96px] border-input/40 bg-transparent px-2 text-xs shadow-none"
                          aria-label={t("table.modelAccessScopeAria", { name: item.platformModelName })}
                        >
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="public" className="text-xs">{t("accessScope.public")}</SelectItem>
                          <SelectItem value="internal" className="text-xs">{t("accessScope.internal")}</SelectItem>
                        </SelectContent>
                      </Select>
                    </div>
                  </TableCell>
                  <TableCell className="whitespace-nowrap py-1.5 text-muted-foreground">
                    {formatDateTime(item.updatedAt, locale)}
                  </TableCell>
                  <TableCell className="w-[56px] whitespace-nowrap py-1.5" stickyEnd>
                    <div className="flex h-7 items-center justify-end">
                      <DropdownMenu modal={false}>
                        <DropdownMenuTrigger asChild>
                          <Button
                            type="button"
                            size="icon-sm"
                            variant="ghost"
                            className="text-muted-foreground shadow-none"
                            aria-label={t("table.modelActions")}
                          >
                            <MoreHorizontal className="size-3.5 stroke-1" />
                          </Button>
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuItem onSelect={() => onEdit(item)}>
                            <Pencil className="size-3.5 stroke-1" />
                            {t("table.editModel")}
                          </DropdownMenuItem>
                          <DropdownMenuSeparator />
                          {item.status === "active" ? (
                            <DropdownMenuItem onSelect={() => onToggleStatus(item, "inactive")}>
                              <CircleOff className="size-3.5 stroke-1" />
                              {t("table.disableModel")}
                            </DropdownMenuItem>
                          ) : (
                            <DropdownMenuItem onSelect={() => onToggleStatus(item, "active")}>
                              <RotateCcw className="size-3.5 stroke-1" />
                              {t("table.enableModel")}
                            </DropdownMenuItem>
                          )}
                          <DropdownMenuSeparator />
                          <DropdownMenuItem
                            onSelect={() => onDelete(item)}
                            className="text-destructive focus:text-destructive"
                          >
                            <Trash2 className="size-3.5 stroke-1" />
                            {t("table.deleteModel")}
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    </div>
                  </TableCell>
                </TableRow>
              );
            })
          : null}
        {showRows ? <VirtualTablePaddingRow colSpan={9} height={virtualRows.paddingBottom} /> : null}
      </TableBody>
    </Table>
  );
}
