"use client";

import { Edit3, Globe2, KeyRound, Plus, RefreshCw, Trash2 } from "lucide-react";
import { useTranslations } from "next-intl";
import * as React from "react";
import { toast } from "sonner";

import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { SpinnerLabel } from "@/components/ui/spinner";
import { Switch } from "@/components/ui/switch";
import { Table, TableBody, TableCell, TableEmptyRow, TableHead, TableHeader, TableLoadingRow, TableRow } from "@/components/ui/table";
import { type AdminRelayConnector, type AdminRelayIngressRoute, createAdminRelayConnector, createAdminRelayRoute, deleteAdminRelayConnector, deleteAdminRelayRoute, listAdminRelayConnectors, listAdminRelayRoutes, type RelayConnectorInput, type RelayRouteInput, updateAdminRelayConnector, updateAdminRelayRoute } from "@/features/admin/api/relays";
import { resolveAdminErrorMessage } from "@/features/admin/utils/admin-error";
import { cn } from "@/lib/utils";
import { resolveAccessToken } from "@/shared/auth/resolve-access-token";

type ConnectorForm = RelayConnectorInput & { id?: string; publicID?: string };
type RouteForm = RelayRouteInput & { id?: number };

const emptyConnector: ConnectorForm = {
  name: "",
  protocol: "sub2api",
  accountBaseURL: "",
  modelBaseURL: "",
  configJSON: "",
  enabled: true,
};

const emptyRoute: RouteForm = { hostname: "", connectorID: "", enabled: true };

function statusClass(enabled: boolean) {
  return enabled
    ? "border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-300"
    : "border-border bg-muted text-muted-foreground";
}

export function AdminRelaysPage() {
  const t = useTranslations("adminRelays");
  const [connectors, setConnectors] = React.useState<AdminRelayConnector[]>([]);
  const [routes, setRoutes] = React.useState<AdminRelayIngressRoute[]>([]);
  const [loading, setLoading] = React.useState(true);
  const [connectorForm, setConnectorForm] = React.useState<ConnectorForm | null>(null);
  const [routeForm, setRouteForm] = React.useState<RouteForm | null>(null);
  const [deleteConnector, setDeleteConnector] = React.useState<AdminRelayConnector | null>(null);
  const [deleteRoute, setDeleteRoute] = React.useState<AdminRelayIngressRoute | null>(null);
  const [saving, setSaving] = React.useState(false);

  const load = React.useCallback(async (showLoading = true) => {
    if (showLoading) setLoading(true);
    try {
      const token = await resolveAccessToken();
      const [nextConnectors, nextRoutes] = await Promise.all([
        listAdminRelayConnectors(token),
        listAdminRelayRoutes(token),
      ]);
      setConnectors(nextConnectors ?? []);
      setRoutes(nextRoutes ?? []);
    } catch (error) {
      toast.error(t("toast.loadFailed"), { description: resolveAdminErrorMessage(error) });
    } finally {
      if (showLoading) setLoading(false);
    }
  }, [t]);

  React.useEffect(() => { void load(); }, [load]);

  async function saveConnector(form: ConnectorForm) {
    if (!form.name.trim() || !form.accountBaseURL.trim()) return;
    setSaving(true);
    try {
      const token = await resolveAccessToken();
      const payload: RelayConnectorInput = {
        name: form.name.trim(), protocol: form.protocol,
        accountBaseURL: form.accountBaseURL.trim(), modelBaseURL: form.modelBaseURL?.trim() || undefined,
        configJSON: form.configJSON?.trim() || undefined, enabled: form.enabled,
      };
      if (form.id) {
        await updateAdminRelayConnector(token, form.id, payload);
        toast.success(t("toast.connectorUpdated"));
      } else {
        await createAdminRelayConnector(token, payload);
        toast.success(t("toast.connectorCreated"));
      }
      setConnectorForm(null);
      await load(false);
    } catch (error) {
      toast.error(t("toast.saveFailed"), { description: resolveAdminErrorMessage(error) });
    } finally {
      setSaving(false);
    }
  }

  async function saveRoute(form: RouteForm) {
    if (!form.hostname.trim() || !form.connectorID) return;
    setSaving(true);
    try {
      const token = await resolveAccessToken();
      const payload: RelayRouteInput = { hostname: form.hostname.trim(), connectorID: form.connectorID, enabled: form.enabled };
      if (form.id) {
        await updateAdminRelayRoute(token, form.id, payload);
        toast.success(t("toast.routeUpdated"));
      } else {
        await createAdminRelayRoute(token, payload);
        toast.success(t("toast.routeCreated"));
      }
      setRouteForm(null);
      await load(false);
    } catch (error) {
      toast.error(t("toast.saveFailed"), { description: resolveAdminErrorMessage(error) });
    } finally {
      setSaving(false);
    }
  }

  async function confirmDeleteConnector() {
    if (!deleteConnector) return;
    setSaving(true);
    try {
      const token = await resolveAccessToken();
      await deleteAdminRelayConnector(token, deleteConnector.publicID);
      toast.success(t("toast.connectorDeleted"));
      setDeleteConnector(null);
      await load(false);
    } catch (error) {
      toast.error(t("toast.deleteFailed"), { description: resolveAdminErrorMessage(error) });
    } finally {
      setSaving(false);
    }
  }

  async function confirmDeleteRoute() {
    if (!deleteRoute) return;
    setSaving(true);
    try {
      const token = await resolveAccessToken();
      await deleteAdminRelayRoute(token, deleteRoute.id);
      toast.success(t("toast.routeDeleted"));
      setDeleteRoute(null);
      await load(false);
    } catch (error) {
      toast.error(t("toast.deleteFailed"), { description: resolveAdminErrorMessage(error) });
    } finally {
      setSaving(false);
    }
  }

  function connectorName(publicID: string) {
    return connectors.find((item) => item.publicID === publicID)?.name ?? publicID;
  }

  return (
    <div className="space-y-6 pb-10">
      <div className="flex min-h-10 items-center justify-between gap-3 px-1">
        <div>
          <h3 className="text-sm font-semibold">{t("pageTitle")}</h3>
          <p className="mt-1 text-xs text-muted-foreground">{t("pageDescription")}</p>
        </div>
        <Button type="button" size="sm" variant="outline" className="h-7 gap-1 text-xs" onClick={() => void load()} disabled={loading} title={t("actions.refresh")}>
          <RefreshCw className={cn("size-3.5", loading && "animate-spin")} />
          {t("actions.refresh")}
        </Button>
      </div>

      <section className="space-y-2">
        <div className="flex items-center justify-between gap-2 px-1">
          <div className="flex items-center gap-2"><KeyRound className="size-4 text-muted-foreground" /><h4 className="text-xs font-semibold">{t("connectors.title")}</h4></div>
          <Button type="button" size="sm" className="h-7 gap-1 text-xs" onClick={() => setConnectorForm({ ...emptyConnector })} disabled={loading}><Plus className="size-3.5" />{t("actions.addConnector")}</Button>
        </div>
        <Table>
          <TableHeader><TableRow className="hover:bg-transparent"><TableHead>{t("connectors.name")}</TableHead><TableHead>{t("connectors.protocol")}</TableHead><TableHead>{t("connectors.account")}</TableHead><TableHead>{t("connectors.status")}</TableHead><TableHead stickyEnd className="w-24" /></TableRow></TableHeader>
          <TableBody>
            {loading ? <TableLoadingRow colSpan={5} /> : null}
            {!loading && connectors.length === 0 ? <TableEmptyRow colSpan={5}>{t("connectors.empty")}</TableEmptyRow> : null}
            {!loading ? connectors.map((item) => (
              <TableRow key={item.publicID}>
                <TableCell><div className="font-medium">{item.name}</div><div className="max-w-[260px] truncate text-[11px] text-muted-foreground">{item.modelBaseURL}</div></TableCell>
                <TableCell className="uppercase text-[11px]">{item.protocol}</TableCell>
                <TableCell className="max-w-[230px] truncate text-muted-foreground">{item.accountBaseURL}</TableCell>
                <TableCell><Badge variant="outline" className={statusClass(item.enabled)}>{item.enabled ? t("status.enabled") : t("status.disabled")}</Badge></TableCell>
                <TableCell stickyEnd className="text-right"><Button size="icon" variant="ghost" title={t("actions.edit")} onClick={() => setConnectorForm({ id: item.publicID, publicID: item.publicID, name: item.name, protocol: "sub2api", accountBaseURL: item.accountBaseURL, modelBaseURL: item.modelBaseURL, enabled: item.enabled })}><Edit3 className="size-3.5" /></Button><Button size="icon" variant="ghost" title={t("actions.delete")} onClick={() => setDeleteConnector(item)}><Trash2 className="size-3.5" /></Button></TableCell>
              </TableRow>
            )) : null}
          </TableBody>
        </Table>
      </section>

      <section className="space-y-2">
        <div className="flex items-center justify-between gap-2 px-1"><div className="flex items-center gap-2"><Globe2 className="size-4 text-muted-foreground" /><h4 className="text-xs font-semibold">{t("routes.title")}</h4></div><Button type="button" size="sm" className="h-7 gap-1 text-xs" onClick={() => setRouteForm({ ...emptyRoute, connectorID: connectors[0]?.publicID ?? "" })} disabled={loading || connectors.length === 0}><Plus className="size-3.5" />{t("actions.addRoute")}</Button></div>
        <Table>
          <TableHeader><TableRow className="hover:bg-transparent"><TableHead>{t("routes.hostname")}</TableHead><TableHead>{t("routes.connector")}</TableHead><TableHead>{t("routes.status")}</TableHead><TableHead stickyEnd className="w-24" /></TableRow></TableHeader>
          <TableBody>
            {loading ? <TableLoadingRow colSpan={4} /> : null}
            {!loading && routes.length === 0 ? <TableEmptyRow colSpan={4}>{t("routes.empty")}</TableEmptyRow> : null}
            {!loading ? routes.map((item) => <TableRow key={item.id}><TableCell className="font-medium">{item.hostname}</TableCell><TableCell>{connectorName(item.connectorID)}</TableCell><TableCell><Badge variant="outline" className={statusClass(item.enabled)}>{item.enabled ? t("status.enabled") : t("status.disabled")}</Badge></TableCell><TableCell stickyEnd className="text-right"><Button size="icon" variant="ghost" title={t("actions.edit")} onClick={() => setRouteForm(item)}><Edit3 className="size-3.5" /></Button><Button size="icon" variant="ghost" title={t("actions.delete")} onClick={() => setDeleteRoute(item)}><Trash2 className="size-3.5" /></Button></TableCell></TableRow>) : null}
          </TableBody>
        </Table>
      </section>

      <ConnectorDialog form={connectorForm} open={connectorForm !== null} saving={saving} onOpenChange={(open) => !open && !saving && setConnectorForm(null)} onChange={setConnectorForm} onSubmit={() => connectorForm && void saveConnector(connectorForm)} t={t} />
      <RouteDialog form={routeForm} open={routeForm !== null} saving={saving} connectors={connectors} onOpenChange={(open) => !open && !saving && setRouteForm(null)} onChange={setRouteForm} onSubmit={() => routeForm && void saveRoute(routeForm)} t={t} />

      <AlertDialog open={deleteConnector !== null} onOpenChange={(open) => !open && !saving && setDeleteConnector(null)}><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>{t("delete.connectorTitle")}</AlertDialogTitle><AlertDialogDescription>{t("delete.connectorDescription")}</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel disabled={saving}>{t("actions.cancel")}</AlertDialogCancel><AlertDialogAction variant="destructive" disabled={saving} onClick={(event) => { event.preventDefault(); void confirmDeleteConnector(); }}>{saving ? <SpinnerLabel>{t("actions.deleting")}</SpinnerLabel> : t("actions.delete")}</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog>
      <AlertDialog open={deleteRoute !== null} onOpenChange={(open) => !open && !saving && setDeleteRoute(null)}><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>{t("delete.routeTitle")}</AlertDialogTitle><AlertDialogDescription>{t("delete.routeDescription")}</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel disabled={saving}>{t("actions.cancel")}</AlertDialogCancel><AlertDialogAction variant="destructive" disabled={saving} onClick={(event) => { event.preventDefault(); void confirmDeleteRoute(); }}>{saving ? <SpinnerLabel>{t("actions.deleting")}</SpinnerLabel> : t("actions.delete")}</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog>
    </div>
  );
}

function ConnectorDialog({ form, open, saving, onOpenChange, onChange, onSubmit, t }: { form: ConnectorForm | null; open: boolean; saving: boolean; onOpenChange: (open: boolean) => void; onChange: React.Dispatch<React.SetStateAction<ConnectorForm | null>>; onSubmit: () => void; t: ReturnType<typeof useTranslations<"adminRelays">> }) {
  const update = (patch: Partial<ConnectorForm>) => onChange((current) => current ? { ...current, ...patch } : current);
  const editing = Boolean(form?.id);
  return <Dialog open={open} onOpenChange={onOpenChange}><DialogContent className="sm:max-w-[560px]"><DialogHeader><DialogTitle>{editing ? t("connectorDialog.editTitle") : t("connectorDialog.createTitle")}</DialogTitle><DialogDescription>{t("connectorDialog.description")}</DialogDescription></DialogHeader>{form ? <form className="grid gap-3" onSubmit={(event) => { event.preventDefault(); onSubmit(); }}><div className="grid gap-1"><Label htmlFor="relay-name">{t("fields.name")}</Label><Input id="relay-name" value={form.name} onChange={(event) => update({ name: event.target.value })} disabled={saving} required /></div><div className="grid gap-1"><Label htmlFor="relay-protocol">{t("fields.protocol")}</Label><Select value={form.protocol} disabled><SelectTrigger id="relay-protocol"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="sub2api">Sub2API</SelectItem></SelectContent></Select></div><div className="grid gap-1"><Label htmlFor="relay-account">{t("fields.accountBaseURL")}</Label><Input id="relay-account" type="url" value={form.accountBaseURL} onChange={(event) => update({ accountBaseURL: event.target.value })} disabled={saving || editing} required /></div><div className="grid gap-1"><Label htmlFor="relay-model">{t("fields.modelBaseURL")}</Label><Input id="relay-model" type="url" value={form.modelBaseURL ?? ""} onChange={(event) => update({ modelBaseURL: event.target.value })} disabled={saving} placeholder={t("fields.sameAsAccount")} /></div><div className="flex items-center justify-between rounded-md bg-muted/30 px-3 py-2"><div><p className="text-xs font-medium">{t("fields.enabled")}</p><p className="text-[11px] text-muted-foreground">{t("fields.enabledDescription")}</p></div><Switch checked={form.enabled} onCheckedChange={(enabled) => update({ enabled })} disabled={saving} /></div><DialogFooter><Button type="button" variant="ghost" onClick={() => onOpenChange(false)} disabled={saving}>{t("actions.cancel")}</Button><Button type="submit" disabled={saving || !form.name.trim() || !form.accountBaseURL.trim()}>{saving ? <SpinnerLabel>{t("actions.saving")}</SpinnerLabel> : t("actions.save")}</Button></DialogFooter></form> : null}</DialogContent></Dialog>;
}

function RouteDialog({ form, open, saving, connectors, onOpenChange, onChange, onSubmit, t }: { form: RouteForm | null; open: boolean; saving: boolean; connectors: AdminRelayConnector[]; onOpenChange: (open: boolean) => void; onChange: React.Dispatch<React.SetStateAction<RouteForm | null>>; onSubmit: () => void; t: ReturnType<typeof useTranslations<"adminRelays">> }) {
  const update = (patch: Partial<RouteForm>) => onChange((current) => current ? { ...current, ...patch } : current);
  return <Dialog open={open} onOpenChange={onOpenChange}><DialogContent className="sm:max-w-[480px]"><DialogHeader><DialogTitle>{form?.id ? t("routeDialog.editTitle") : t("routeDialog.createTitle")}</DialogTitle><DialogDescription>{t("routeDialog.description")}</DialogDescription></DialogHeader>{form ? <form className="grid gap-3" onSubmit={(event) => { event.preventDefault(); onSubmit(); }}><div className="grid gap-1"><Label htmlFor="relay-hostname">{t("fields.hostname")}</Label><Input id="relay-hostname" value={form.hostname} onChange={(event) => update({ hostname: event.target.value })} disabled={saving} placeholder="chat.example.com" required /></div><div className="grid gap-1"><Label htmlFor="relay-connector">{t("fields.connector")}</Label><Select value={form.connectorID} onValueChange={(connectorID) => update({ connectorID })} disabled={saving}><SelectTrigger id="relay-connector"><SelectValue placeholder={t("fields.selectConnector")} /></SelectTrigger><SelectContent>{connectors.map((item) => <SelectItem key={item.publicID} value={item.publicID}>{item.name}</SelectItem>)}</SelectContent></Select></div><div className="flex items-center justify-between rounded-md bg-muted/30 px-3 py-2"><div><p className="text-xs font-medium">{t("fields.enabled")}</p><p className="text-[11px] text-muted-foreground">{t("fields.routeEnabledDescription")}</p></div><Switch checked={form.enabled} onCheckedChange={(enabled) => update({ enabled })} disabled={saving} /></div><DialogFooter><Button type="button" variant="ghost" onClick={() => onOpenChange(false)} disabled={saving}>{t("actions.cancel")}</Button><Button type="submit" disabled={saving || !form.hostname.trim() || !form.connectorID}>{saving ? <SpinnerLabel>{t("actions.saving")}</SpinnerLabel> : t("actions.save")}</Button></DialogFooter></form> : null}</DialogContent></Dialog>;
}
