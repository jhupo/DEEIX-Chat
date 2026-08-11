"use client";

import * as React from "react";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { SpinnerLabel } from "@/components/ui/spinner";
import { useLocalizedErrorMessage } from "@/i18n/use-localized-error";
import { createSub2RemoteKey, listSub2KeyGroups, type Sub2KeyGroupDTO } from "@/shared/api/sub2-key";
import { useAuthSession } from "@/shared/auth/auth-session-context";
import { createIdempotencyKey } from "@/shared/lib/idempotency-key";

export function SubscriptionAPIKeyDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const t = useTranslations("settings.subscriptionPage.apiKey");
  const resolveErrorMessage = useLocalizedErrorMessage();
  const { accessToken } = useAuthSession();
  const [groups, setGroups] = React.useState<Sub2KeyGroupDTO[]>([]);
  const [name, setName] = React.useState("");
  const [groupID, setGroupID] = React.useState("");
  const [loading, setLoading] = React.useState(false);
  const [creating, setCreating] = React.useState(false);

  React.useEffect(() => {
    if (!open) return;
    let cancelled = false;
    setName(t("defaultName"));
    setGroups([]);
    setGroupID("");
    setLoading(true);
    void listSub2KeyGroups(accessToken)
      .then((items) => {
        if (cancelled) return;
        setGroups(items);
      })
      .catch((error) => {
        if (!cancelled) toast.error(t("loadFailed"), { description: resolveErrorMessage(error) });
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => { cancelled = true; };
  }, [accessToken, open, resolveErrorMessage, t]);

  const submit = React.useCallback(async () => {
    const trimmedName = name.trim();
    const selectedGroupID = Number(groupID);
    if (!trimmedName || !Number.isSafeInteger(selectedGroupID) || selectedGroupID <= 0) return;
    setCreating(true);
    try {
      await createSub2RemoteKey(accessToken, { name: trimmedName, groupID: selectedGroupID }, createIdempotencyKey());
      toast.success(t("created"));
      onOpenChange(false);
    } catch (error) {
      toast.error(t("createFailed"), { description: resolveErrorMessage(error) });
    } finally {
      setCreating(false);
    }
  }, [accessToken, groupID, name, onOpenChange, resolveErrorMessage, t]);

  return (
    <Dialog open={open} onOpenChange={(nextOpen) => { if (!creating) onOpenChange(nextOpen); }}>
      <DialogContent className="sm:max-w-[420px]">
        <form className="grid gap-4" onSubmit={(event) => { event.preventDefault(); void submit(); }}>
          <DialogHeader>
            <DialogTitle>{t("title")}</DialogTitle>
            <DialogDescription>{t("description")}</DialogDescription>
          </DialogHeader>
          <div className="grid gap-4">
            <label className="grid gap-1.5 text-xs font-medium" htmlFor="sub2-key-name">
              {t("name")}
              <Input id="sub2-key-name" value={name} maxLength={100} disabled={creating} onChange={(event) => setName(event.target.value)} />
            </label>
            <div className="grid gap-1.5 text-xs font-medium">
              <label htmlFor="sub2-key-group">{t("group")}</label>
              <Select value={groupID} onValueChange={setGroupID} disabled={loading || creating || groups.length === 0}>
                <SelectTrigger id="sub2-key-group" aria-label={t("group")}>
                  <SelectValue placeholder={loading ? t("loading") : groups.length === 0 ? t("groupEmpty") : t("groupPlaceholder")} />
                </SelectTrigger>
                <SelectContent>
                  {groups.map((group) => (
                    <SelectItem key={group.id} value={String(group.id)}>
                      {group.name} · {group.platform}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          <DialogFooter>
            <Button type="button" variant="ghost" disabled={creating} onClick={() => onOpenChange(false)}>{t("cancel")}</Button>
            <Button type="submit" disabled={loading || creating || !name.trim() || !groupID}>
              {creating ? <SpinnerLabel>{t("creating")}</SpinnerLabel> : t("create")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
