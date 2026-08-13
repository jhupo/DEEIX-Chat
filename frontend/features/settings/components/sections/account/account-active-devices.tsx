"use client";

import { MoreHorizontal, Plus } from "lucide-react";
import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Table, TableBody, TableCell, TableEmptyRow, TableHead, TableHeader, TableLoadingRow, TableRow } from "@/components/ui/table";
import { formatDateTime } from "@/features/settings/model/account-settings";
import { useAppLocale } from "@/i18n/app-i18n-provider";
import type { AgentDeviceDTO } from "@/shared/api/agent-gateway";
import { SettingsSection } from "@/shared/components/settings-layout";

export function AccountActiveDevicesSection({
  devices,
  loading,
  revokingDeviceId,
  onAdd,
  addDisabled,
  onRevoke,
}: {
  devices: AgentDeviceDTO[];
  loading: boolean;
  revokingDeviceId: string;
  onAdd: () => void;
  addDisabled: boolean;
  onRevoke: (device: AgentDeviceDTO) => void;
}) {
  const t = useTranslations("settings.accountPage.device");
  const { locale } = useAppLocale();
  const activeDevices = devices.filter((item) => item.status === "active");

  return (
    <SettingsSection
      title={t("title")}
      actions={(
        <Button type="button" variant="outline" size="sm" disabled={addDisabled} onClick={onAdd}>
          <Plus className="size-3.5 stroke-1" />
          {t("add")}
        </Button>
      )}
    >
      <Table className="table-fixed" style={{ minWidth: 720 }}>
        <colgroup>
          <col style={{ width: 240 }} />
          <col style={{ width: 120 }} />
          <col style={{ width: 132 }} />
          <col style={{ width: 180 }} />
          <col style={{ width: 56 }} />
        </colgroup>
        <TableHeader>
          <TableRow>
            <TableHead>{t("name")}</TableHead>
            <TableHead>{t("status")}</TableHead>
            <TableHead>{t("platform")}</TableHead>
            <TableHead>{t("lastSeen")}</TableHead>
            <TableHead className="w-[56px]" stickyEnd />
          </TableRow>
        </TableHeader>
        <TableBody>
          {loading && activeDevices.length === 0 ? <TableLoadingRow colSpan={5} /> : null}
          {!loading && activeDevices.length === 0 ? <TableEmptyRow colSpan={5}>{t("empty")}</TableEmptyRow> : null}
          {activeDevices.map((device) => (
            <TableRow key={device.deviceId}>
              <TableCell className="max-w-0">
                <span className="block truncate font-medium" title={device.name}>{device.name}</span>
              </TableCell>
              <TableCell>
                <span className="inline-flex items-center gap-1.5 text-xs">
                  <span className={device.online ? "size-1.5 rounded-full bg-emerald-500" : "size-1.5 rounded-full bg-muted-foreground/45"} />
                  {device.online ? t("online") : t("offline")}
                </span>
              </TableCell>
              <TableCell className="text-muted-foreground">{t(`platforms.${device.platform}`)}</TableCell>
              <TableCell className="max-w-0 text-muted-foreground">
                <span className="block truncate">{device.lastSeenAt ? formatDateTime(device.lastSeenAt, locale) : "-"}</span>
              </TableCell>
              <TableCell className="w-[56px]" stickyEnd>
                <div className="flex justify-end">
                  <DropdownMenu modal={false}>
                    <DropdownMenuTrigger asChild>
                      <Button type="button" variant="ghost" size="icon" className="size-8" aria-label={t("actions")}>
                        <MoreHorizontal className="size-3.5 stroke-1" />
                      </Button>
                    </DropdownMenuTrigger>
                    <DropdownMenuContent align="end">
                      <DropdownMenuItem
                        disabled={revokingDeviceId === device.deviceId}
                        onClick={() => onRevoke(device)}
                      >
                        {t("revoke")}
                      </DropdownMenuItem>
                    </DropdownMenuContent>
                  </DropdownMenu>
                </div>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </SettingsSection>
  );
}
