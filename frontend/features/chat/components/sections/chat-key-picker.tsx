"use client";

import { Check, KeyRound, RefreshCw, Trash2 } from "lucide-react";
import { useTranslations } from "next-intl";

import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { InputGroupButton } from "@/components/ui/input-group";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import type { Sub2KeyBindingDTO, Sub2RemoteKeyDTO } from "@/shared/api/sub2-key";

type ChatKeyPickerProps = {
  remoteKeys: Sub2RemoteKeyDTO[];
  bindings: Sub2KeyBindingDTO[];
  selectedKeyBindingID: string;
  loading: boolean;
  error: string;
  onRefresh: () => void | Promise<void>;
  onSelect: (remoteKeyID: number) => void | Promise<void>;
  onDelete: (publicID: string) => void | Promise<void>;
};

export function ChatKeyPicker({ remoteKeys, bindings, selectedKeyBindingID, loading, error, onRefresh, onSelect, onDelete }: ChatKeyPickerProps) {
  const t = useTranslations("chat.keyPicker");
  const selected = bindings.find((item) => item.publicID === selectedKeyBindingID);
  const boundRemoteIDs = new Set(bindings.map((item) => item.remoteKeyID));
  const unbound = remoteKeys.filter((item) => !boundRemoteIDs.has(item.remoteKeyID));
  const label = selected?.maskedKey || selected?.label || t("select");
  return (
    <DropdownMenu modal={false}>
      <Tooltip><TooltipTrigger asChild><DropdownMenuTrigger asChild>
        <InputGroupButton type="button" variant="ghost" size="sm" className="h-7 min-w-7 rounded-md px-1.5 text-muted-foreground hover:text-foreground sm:h-8 sm:max-w-32 sm:px-2" disabled={loading} aria-label={t("select")}>
          <KeyRound className="size-4 shrink-0" strokeWidth={1.7} /><span className="hidden min-w-0 truncate text-[12px] font-medium sm:inline">{label}</span>
        </InputGroupButton>
      </DropdownMenuTrigger></TooltipTrigger><TooltipContent side="top" className="text-xs">{label}</TooltipContent></Tooltip>
      <DropdownMenuContent align="end" side="top" className="w-64">
        <div className="flex items-center justify-between px-2 py-1"><DropdownMenuLabel className="p-0 text-[11px]">{t("title")}</DropdownMenuLabel><Tooltip><TooltipTrigger asChild><InputGroupButton type="button" variant="ghost" size="icon-sm" className="size-6" aria-label={t("refresh")} onClick={() => void onRefresh()}><RefreshCw className="size-3.5" /></InputGroupButton></TooltipTrigger><TooltipContent side="top" className="text-xs">{t("refresh")}</TooltipContent></Tooltip></div>
        {error ? <p className="px-2 py-1 text-[11px] text-destructive">{t("loadFailed")}</p> : null}
        {bindings.map((item) => (
          <div key={item.publicID} role="presentation" className="flex items-center gap-1">
            <DropdownMenuItem onSelect={() => void onSelect(item.remoteKeyID)} className="min-w-0 flex-1 gap-2">
              <span className="min-w-0 flex-1 truncate">{item.maskedKey || item.label}</span>
              {item.publicID === selectedKeyBindingID ? <Check className="size-3.5" /> : null}
            </DropdownMenuItem>
            <Tooltip>
              <TooltipTrigger asChild>
                <DropdownMenuItem
                  className="shrink-0 px-1.5"
                  aria-label={t("delete")}
                  onSelect={(event) => {
                    event.preventDefault();
                    void onDelete(item.publicID);
                  }}
                >
                  <Trash2 className="size-3.5" />
                </DropdownMenuItem>
              </TooltipTrigger>
              <TooltipContent side="left" className="text-xs">{t("delete")}</TooltipContent>
            </Tooltip>
          </div>
        ))}
        {bindings.length > 0 && unbound.length > 0 ? <DropdownMenuSeparator /> : null}
        {unbound.map((item) => <DropdownMenuItem key={item.remoteKeyID} onSelect={() => void onSelect(item.remoteKeyID)}><KeyRound className="size-3.5" /><span className="min-w-0 truncate">{item.maskedKey || item.label}</span></DropdownMenuItem>)}
        {!loading && bindings.length === 0 && unbound.length === 0 ? <p className="px-2 py-3 text-[11px] text-muted-foreground">{t("empty")}</p> : null}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
