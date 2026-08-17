"use client";

import * as React from "react";
import { MoreHorizontal, Plus, RefreshCw } from "lucide-react";
import { useTranslations } from "next-intl";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
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
  updatingDeviceId,
  onAdd,
  addDisabled,
  onRevoke,
  onUpdate,
}: {
  devices: AgentDeviceDTO[];
  loading: boolean;
  revokingDeviceId: string;
  updatingDeviceId: string;
  onAdd: () => void;
  addDisabled: boolean;
  onRevoke: (device: AgentDeviceDTO) => void;
  onUpdate: (device: AgentDeviceDTO) => void;
}) {
  const t = useTranslations("settings.accountPage.device");
  const { locale } = useAppLocale();
  const activeDevices = devices.filter((item) => item.status === "active");
  const [revokeTarget, setRevokeTarget] = React.useState<AgentDeviceDTO | null>(null);

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
      <Table className="table-fixed" style={{ minWidth: 840 }}>
        <colgroup>
          <col style={{ width: 240 }} />
          <col style={{ width: 120 }} />
          <col style={{ width: 150 }} />
          <col style={{ width: 132 }} />
          <col style={{ width: 180 }} />
          <col style={{ width: 56 }} />
        </colgroup>
        <TableHeader>
          <TableRow>
            <TableHead>{t("name")}</TableHead>
            <TableHead>{t("status")}</TableHead>
            <TableHead>{t("version")}</TableHead>
            <TableHead>{t("platform")}</TableHead>
            <TableHead>{t("lastSeen")}</TableHead>
            <TableHead className="w-[56px]" stickyEnd />
          </TableRow>
        </TableHeader>
        <TableBody>
          {loading && activeDevices.length === 0 ? <TableLoadingRow colSpan={6} /> : null}
          {!loading && activeDevices.length === 0 ? <TableEmptyRow colSpan={6}>{t("empty")}</TableEmptyRow> : null}
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
              <TableCell className="max-w-0">
                <span className="block truncate font-mono text-xs" title={device.agentVersion || t("versionUnknown")}>
                  {device.agentVersion || "-"}
                </span>
                {device.updateAvailable ? (
                  <span className="block truncate text-[11px] text-muted-foreground">
                    {t("latestVersion", { version: device.latestAgentVersion })}
                  </span>
                ) : null}
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
                        disabled={!device.updateAvailable || updatingDeviceId === device.deviceId}
                        onClick={() => onUpdate(device)}
                      >
                        <RefreshCw className={updatingDeviceId === device.deviceId ? "animate-spin" : ""} />
                        {updatingDeviceId === device.deviceId ? t("updating") : t("update")}
                      </DropdownMenuItem>
                      <DropdownMenuItem
                        disabled={revokingDeviceId === device.deviceId}
                        onClick={() => setRevokeTarget(device)}
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
      <AlertDialog open={revokeTarget !== null} onOpenChange={(open) => !open && setRevokeTarget(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("confirmTitle")}</AlertDialogTitle>
            <AlertDialogDescription>
              {t("confirmDescription", { name: revokeTarget?.name || t("name") })}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("cancel")}</AlertDialogCancel>
            <AlertDialogAction
              variant="destructive"
              disabled={!revokeTarget || revokingDeviceId === revokeTarget.deviceId}
              onClick={() => {
                if (revokeTarget) {
                  onRevoke(revokeTarget);
                }
                setRevokeTarget(null);
              }}
            >
              {t("confirm")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </SettingsSection>
  );
}
