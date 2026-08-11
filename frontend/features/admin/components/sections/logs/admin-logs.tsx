"use client";

import { useLocale, useTranslations } from "next-intl";

import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
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
import {
  useAdminConversationEvents,
  useAdminLogs,
  useAdminSecurityLogs,
  useAdminSystemEvents,
} from "@/features/admin/hooks/use-admin-logs";

type LogRecord = Record<string, unknown>;

function formatDateTime(value: unknown, locale: string): string {
  if (typeof value !== "string" || !value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "-";
  return new Intl.DateTimeFormat(locale, {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}

function formatValue(value: unknown, locale: string): string {
  if (value === null || value === undefined || value === "") return "-";
  if (typeof value === "string") return value;
  if (typeof value === "number") return new Intl.NumberFormat(locale).format(value);
  if (typeof value === "boolean") return value ? "True" : "False";
  return JSON.stringify(value);
}

function LogTable({
  rows,
  total,
  page,
  pageCount,
  pageSize,
  loading,
  query,
  setQuery,
  onLoad,
  columns,
  emptyLabel,
}: {
  rows: object[];
  total: number;
  page: number;
  pageCount: number;
  pageSize: number;
  loading: boolean;
  query: string;
  setQuery: (value: string) => void;
  onLoad: (page?: number, pageSize?: number) => Promise<void>;
  columns: Array<{ key: string; label: string; date?: boolean }>;
  emptyLabel: string;
}) {
  const t = useTranslations("adminLogs");
  const locale = useLocale();

  return (
    <div className="space-y-3">
      <TableToolbar
        query={query}
        onQueryChange={setQuery}
        queryPlaceholder={t("audit.searchPlaceholder")}
        loading={loading}
        onRefresh={() => void onLoad(page, pageSize)}
      />
      <Table viewportClassName="max-h-[calc(100svh-16rem)] overflow-y-auto">
        <TableHeader>
          <TableRow className="hover:bg-transparent">
            {columns.map((column) => <TableHead key={column.key}>{column.label}</TableHead>)}
          </TableRow>
        </TableHeader>
        <TableBody>
          {loading && rows.length === 0 ? <TableLoadingRow colSpan={columns.length} /> : null}
          {!loading && rows.length === 0 ? <TableEmptyRow colSpan={columns.length}>{emptyLabel}</TableEmptyRow> : null}
          {rows.map((item, index) => {
            const record = item as LogRecord;
            return (
              <TableRow key={String(record.id ?? index)}>
                {columns.map((column) => (
                  <TableCell key={column.key} className="max-w-[18rem] truncate text-xs text-muted-foreground" title={formatValue(record[column.key], locale)}>
                    {column.date ? formatDateTime(record[column.key], locale) : formatValue(record[column.key], locale)}
                  </TableCell>
                ))}
              </TableRow>
            );
          })}
        </TableBody>
      </Table>
      <TablePagination
        total={total}
        page={page}
        pageCount={pageCount}
        pageSize={pageSize}
        loading={loading}
        onPageChange={(nextPage) => void onLoad(nextPage, pageSize)}
        onPageSizeChange={(nextPageSize) => void onLoad(1, nextPageSize)}
      />
    </div>
  );
}

function AuditLogs() {
  const logs = useAdminLogs();
  return <LogTable {...logs} rows={logs.auditLogs} onLoad={logs.loadAuditLogs} emptyLabel="No audit events" columns={[
    { key: "id", label: "ID" }, { key: "actorLabel", label: "Actor" }, { key: "action", label: "Action" },
    { key: "resource", label: "Resource" }, { key: "ip", label: "IP" }, { key: "createdAt", label: "Time", date: true },
  ]} />;
}

function AuthLogs() {
  const logs = useAdminSecurityLogs();
  return <LogTable {...logs} rows={logs.sortedEvents} onLoad={logs.loadSecurityLogs} emptyLabel="No authentication events" columns={[
    { key: "id", label: "ID" }, { key: "userLabel", label: "User" }, { key: "eventType", label: "Event" },
    { key: "result", label: "Result" }, { key: "clientIP", label: "IP" }, { key: "occurredAt", label: "Time", date: true },
  ]} />;
}

function SystemLogs() {
  const logs = useAdminSystemEvents();
  return <LogTable {...logs} rows={logs.events} onLoad={logs.loadSystemEvents} emptyLabel="No system events" columns={[
    { key: "id", label: "ID" }, { key: "level", label: "Level" }, { key: "source", label: "Source" },
    { key: "event", label: "Event" }, { key: "message", label: "Message" }, { key: "createdAt", label: "Time", date: true },
  ]} />;
}

function ConversationLogs() {
  const logs = useAdminConversationEvents();
  return <LogTable {...logs} rows={logs.events} onLoad={logs.loadConversationEvents} emptyLabel="No conversation events" columns={[
    { key: "id", label: "ID" }, { key: "runID", label: "Run" }, { key: "eventScope", label: "Scope" },
    { key: "eventType", label: "Event" }, { key: "status", label: "Status" }, { key: "createdAt", label: "Time", date: true },
  ]} />;
}

export function AdminLogsPage() {
  const t = useTranslations("adminLogs");
  return (
    <div className="space-y-5 pb-10">
      <h3 className="px-1 text-sm font-semibold">{t("centerTitle")}</h3>
      <Tabs defaultValue="audit" className="space-y-3">
        <TabsList variant="line">
          <TabsTrigger value="audit">{t("tabs.audit")}</TabsTrigger>
          <TabsTrigger value="auth">{t("tabs.auth")}</TabsTrigger>
          <TabsTrigger value="system">System</TabsTrigger>
          <TabsTrigger value="conversation">{t("tabs.conversation")}</TabsTrigger>
        </TabsList>
        <TabsContent value="audit"><AuditLogs /></TabsContent>
        <TabsContent value="auth"><AuthLogs /></TabsContent>
        <TabsContent value="system"><SystemLogs /></TabsContent>
        <TabsContent value="conversation"><ConversationLogs /></TabsContent>
      </Tabs>
    </div>
  );
}
