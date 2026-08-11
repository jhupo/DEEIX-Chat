"use client";

import * as React from "react";
import { useLocale, useTranslations } from "next-intl";
import { toast } from "sonner";

import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
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
import { TablePagination, TableToolbar } from "@/components/ui/table-tools";
import { revokeAdminUserSessions } from "@/features/admin/api";
import type { AdminUserDTO } from "@/features/admin/api/admin.types";
import { AccountConfirmationDialog } from "@/features/admin/components/sections/accounts/accounts-confirm-dialog";
import { resolveAdminErrorMessage } from "@/features/admin/utils/admin-error";
import { formatDateTime, resolveUserInitial, resolveValue } from "@/features/admin/utils/account-display";
import { resolveAccessToken } from "@/shared/auth/resolve-access-token";
import { resolveAvatarImageSrc } from "@/shared/lib/avatar";

type AccountsUsersProps = {
  items: AdminUserDTO[];
  total: number;
  page: number;
  setPage: (value: number) => void;
  pageSize: number;
  setPageSize: (value: number) => void;
  pageCount: number;
  query: string;
  setQuery: (value: string) => void;
  loading: boolean;
  onLoadUsers: () => Promise<void>;
};

export function AccountsUsers({
  items,
  total,
  page,
  setPage,
  pageSize,
  setPageSize,
  pageCount,
  query,
  setQuery,
  loading,
  onLoadUsers,
}: AccountsUsersProps) {
  const t = useTranslations("adminUsers");
  const locale = useLocale();
  const [revokeTarget, setRevokeTarget] = React.useState<AdminUserDTO | null>(null);
  const [revokePending, setRevokePending] = React.useState(false);

  const handleRevokeSessions = React.useCallback(async () => {
    if (!revokeTarget || revokePending) {
      return;
    }

    setRevokePending(true);
    try {
      const token = await resolveAccessToken();
      if (!token) {
        toast.error(t("toast.sessionExpired"), { description: t("toast.signInAgain") });
        return;
      }
      await revokeAdminUserSessions(token, revokeTarget.id);
      toast.success(t("toast.sessionsRevoked"));
      setRevokeTarget(null);
      await onLoadUsers();
    } catch (error) {
      toast.error(t("toast.revokeSessionsFailed"), { description: resolveAdminErrorMessage(error) });
    } finally {
      setRevokePending(false);
    }
  }, [onLoadUsers, revokePending, revokeTarget, t]);

  return (
    <>
      <TableToolbar
        query={query}
        onQueryChange={setQuery}
        queryPlaceholder={t("table.searchPlaceholder")}
        loading={loading || revokePending}
        onRefresh={() => void onLoadUsers()}
        refreshDisabled={loading || revokePending}
        refreshLoading={loading}
      />

      <Table shellClassName="rounded-md" viewportClassName="max-h-[calc(100svh-16rem)] overflow-y-auto">
        <TableHeader>
          <TableRow className="hover:bg-transparent">
            <TableHead>{t("fields.info")}</TableHead>
            <TableHead className="w-[220px]">{t("editor.email")}</TableHead>
            <TableHead className="w-[170px]">{t("fields.lastActive")}</TableHead>
            <TableHead className="w-[120px] text-right">{t("editor.revokeSessions")}</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {loading ? <TableLoadingRow colSpan={4} /> : null}
          {!loading && items.length === 0 ? <TableEmptyRow colSpan={4}>{t("table.empty")}</TableEmptyRow> : null}
          {!loading
            ? items.map((item) => {
                const label = item.displayName.trim() || item.username.trim() || item.publicID.trim() || t("fallbackUser");
                return (
                  <TableRow key={item.id}>
                    <TableCell className="py-2">
                      <div className="flex min-w-0 items-center gap-3">
                        <Avatar className="size-7 shrink-0">
                          <AvatarImage src={resolveAvatarImageSrc(item.avatarURL, item) || undefined} alt={label} />
                          <AvatarFallback className="bg-foreground text-xs font-medium text-background">
                            {resolveUserInitial(item)}
                          </AvatarFallback>
                        </Avatar>
                        <div className="min-w-0">
                          <p className="truncate text-xs font-medium text-foreground">{label}</p>
                          <p className="truncate text-xs text-muted-foreground">{item.publicID || `#${item.id}`}</p>
                        </div>
                      </div>
                    </TableCell>
                    <TableCell className="max-w-[220px] truncate py-2 text-xs text-muted-foreground" title={item.email}>
                      {resolveValue(item.email)}
                    </TableCell>
                    <TableCell className="py-2 text-xs text-muted-foreground">
                      {formatDateTime(item.lastActiveAt || item.lastLoginAt, locale)}
                    </TableCell>
                    <TableCell className="py-2 text-right">
                      <Button
                        type="button"
                        size="sm"
                        variant="ghost"
                        className="h-7 px-2 text-xs text-muted-foreground shadow-none hover:bg-muted hover:text-foreground"
                        disabled={revokePending}
                        onClick={() => setRevokeTarget(item)}
                      >
                        {t("editor.revokeSessions")}
                      </Button>
                    </TableCell>
                  </TableRow>
                );
              })
            : null}
        </TableBody>
      </Table>

      <TablePagination
        total={total}
        page={page}
        pageCount={pageCount}
        pageSize={pageSize}
        onPageChange={setPage}
        onPageSizeChange={setPageSize}
        loading={loading || revokePending}
      />

      <AccountConfirmationDialog
        open={revokeTarget !== null}
        onOpenChange={(open) => !open && setRevokeTarget(null)}
        pending={revokePending}
        title={t("confirm.revokeSessionsTitle")}
        description={t("confirm.revokeSessionsDescription")}
        confirmLabel={t("confirm.revoke")}
        pendingLabel={t("confirm.revoking")}
        onConfirm={() => void handleRevokeSessions()}
      />
    </>
  );
}
