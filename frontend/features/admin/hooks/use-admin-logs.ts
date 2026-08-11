import * as React from "react";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import {
  listAdminAuditLogs,
  listAdminConversationEvents,
  listAdminSystemEvents,
  listAdminUserAuthEvents,
} from "@/features/admin/api";
import type {
  AdminConversationEventDTO,
  AdminSystemEventDTO,
  AdminUserAuthEventDTO,
} from "@/features/admin/api/admin.types";
import { resolveAccessToken } from "@/shared/auth/resolve-access-token";
import { resolveAdminErrorMessage } from "@/features/admin/utils/admin-error";

export const ADMIN_LOGS_PAGE_SIZE = 25;

export const AUDIT_LOG_SORT_OPTIONS = [
  { labelKey: "sort.idDesc", value: "id_desc" },
  { labelKey: "sort.idAsc", value: "id_asc" },
  { labelKey: "sort.createdDesc", value: "created_desc" },
  { labelKey: "sort.createdAsc", value: "created_asc" },
] as const;

export const SECURITY_LOG_SORT_OPTIONS = [
  { labelKey: "sort.occurredDesc", value: "occurred_desc" },
  { labelKey: "sort.occurredAsc", value: "occurred_asc" },
  { labelKey: "sort.idDesc", value: "id_desc" },
  { labelKey: "sort.idAsc", value: "id_asc" },
] as const;

export const SYSTEM_EVENT_SORT_OPTIONS = [
  { labelKey: "sort.createdDesc", value: "created_desc" },
  { labelKey: "sort.createdAsc", value: "created_asc" },
  { labelKey: "sort.idDesc", value: "id_desc" },
  { labelKey: "sort.idAsc", value: "id_asc" },
] as const;

export const CONVERSATION_EVENT_SORT_OPTIONS = [
  { labelKey: "sort.createdDesc", value: "created_desc" },
  { labelKey: "sort.createdAsc", value: "created_asc" },
  { labelKey: "sort.latencyDesc", value: "latency_desc" },
  { labelKey: "sort.sequenceAsc", value: "seq_asc" },
] as const;

export type AuditLogSortValue = (typeof AUDIT_LOG_SORT_OPTIONS)[number]["value"];
export type SecurityLogSortValue = (typeof SECURITY_LOG_SORT_OPTIONS)[number]["value"];
export type SystemEventSortValue = (typeof SYSTEM_EVENT_SORT_OPTIONS)[number]["value"];
export type ConversationEventSortValue = (typeof CONVERSATION_EVENT_SORT_OPTIONS)[number]["value"];

type PagedLogState<T> = {
  events: T[];
  total: number;
  page: number;
  pageSize: number;
  pageCount: number;
  loading: boolean;
  query: string;
  setQuery: (value: string) => void;
  load: (page?: number, pageSize?: number) => Promise<void>;
};

function usePagedLogState<T>(
  loadPage: (accessToken: string, page: number, pageSize: number, query: string) => Promise<{ results: T[]; total: number }>,
  errorKey: string,
): PagedLogState<T> {
  const t = useTranslations("adminLogs");
  const [events, setEvents] = React.useState<T[]>([]);
  const [total, setTotal] = React.useState(0);
  const [page, setPage] = React.useState(1);
  const [pageSize, setPageSize] = React.useState(ADMIN_LOGS_PAGE_SIZE);
  const [query, setQueryState] = React.useState("");
  const [debouncedQuery, setDebouncedQuery] = React.useState("");
  const [loading, setLoading] = React.useState(true);
  const requestSeqRef = React.useRef(0);

  React.useEffect(() => {
    const timer = window.setTimeout(() => setDebouncedQuery(query.trim()), 250);
    return () => window.clearTimeout(timer);
  }, [query]);

  const load = React.useCallback(async (nextPage = 1, nextPageSize = pageSize) => {
    const requestSeq = requestSeqRef.current + 1;
    requestSeqRef.current = requestSeq;
    setLoading(true);
    try {
      const accessToken = await resolveAccessToken();
      if (!accessToken) {
        toast.error(t("toast.sessionExpired"), { description: t("toast.signInAgain") });
        return;
      }
      const data = await loadPage(accessToken, nextPage, nextPageSize, debouncedQuery);
      if (requestSeq !== requestSeqRef.current) return;
      setEvents(data.results);
      setTotal(data.total);
      setPage(nextPage);
      setPageSize(nextPageSize);
    } catch (error) {
      toast.error(t(errorKey), { description: resolveAdminErrorMessage(error) });
    } finally {
      if (requestSeq === requestSeqRef.current) setLoading(false);
    }
  }, [debouncedQuery, errorKey, loadPage, pageSize, t]);

  React.useEffect(() => {
    void load(1);
  }, [load]);

  const setQuery = React.useCallback((value: string) => {
    setQueryState(value);
    setPage(1);
  }, []);

  return {
    events,
    total,
    page,
    pageSize,
    pageCount: Math.max(1, Math.ceil(total / pageSize)),
    loading,
    query,
    setQuery,
    load,
  };
}

export function useAdminLogs() {
  const loadPage = React.useCallback((accessToken: string, page: number, pageSize: number, query: string) => (
    listAdminAuditLogs(accessToken, { page, pageSize, query })
  ), []);
  const state = usePagedLogState(loadPage, "toast.auditLoadFailed");
  return { ...state, auditLogs: state.events, loadAuditLogs: state.load };
}

export function useAdminSecurityLogs() {
  const loadPage = React.useCallback((accessToken: string, page: number, pageSize: number) => (
    listAdminUserAuthEvents(accessToken, { page, pageSize })
  ), []);
  const state = usePagedLogState<AdminUserAuthEventDTO>(loadPage, "toast.authLoadFailed");
  return { ...state, sortedEvents: state.events, loadSecurityLogs: state.load };
}

export function useAdminSystemEvents() {
  const loadPage = React.useCallback((accessToken: string, page: number, pageSize: number, query: string) => (
    listAdminSystemEvents(accessToken, { page, pageSize, query })
  ), []);
  const state = usePagedLogState<AdminSystemEventDTO>(loadPage, "toast.systemLoadFailed");
  return { ...state, loadSystemEvents: state.load };
}

export function useAdminConversationEvents() {
  const loadPage = React.useCallback((accessToken: string, page: number, pageSize: number, query: string) => (
    listAdminConversationEvents(accessToken, { page, pageSize, query })
  ), []);
  const state = usePagedLogState<AdminConversationEventDTO>(loadPage, "toast.conversationEventsLoadFailed");
  return { ...state, loadConversationEvents: state.load };
}
