"use client";

import * as React from "react";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import {
  listAgentRuntimeProfiles,
  registerAgentWorkspace,
  renameAgentWorkspace,
  waitForAgentCommand,
} from "@/shared/api/agent-gateway";
import { resolveAccessToken } from "@/shared/auth/resolve-access-token";

export function WorkspaceDialog({
  deviceId,
  open,
  onOpenChange,
  onQueued,
}: {
  deviceId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onQueued: () => void | Promise<void>;
}) {
  const t = useTranslations("recent.projects.workspace");
  const [path, setPath] = React.useState("");
  const [create, setCreate] = React.useState(false);
  const [submitting, setSubmitting] = React.useState(false);
  const pathInputID = React.useId();
  const createInputID = React.useId();

  React.useEffect(() => {
    if (!open) {
      setPath("");
      setCreate(false);
      setSubmitting(false);
    }
  }, [open]);

  const submit = React.useCallback(async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const normalizedPath = path.trim();
    if (!normalizedPath || !deviceId || submitting) return;
    setSubmitting(true);
    try {
      const token = await resolveAccessToken();
      if (!token) throw new Error("missing access token");
      const profiles = await listAgentRuntimeProfiles(token, deviceId);
      const profile = profiles.find((item) =>
        item.status === "ready" &&
        item.provider === "codex" &&
        item.manifest.commands?.includes("workspace.register"),
      );
      if (!profile) {
        toast.error(t("runtimeUnavailable"));
        return;
      }
      const queued = await registerAgentWorkspace(token, deviceId, {
        profileId: profile.profileId,
        path: normalizedPath,
        create,
      });
      const command = await waitForAgentCommand(token, queued.commandId);
      if (!command) throw new Error(t("timeout"));
      if (command.status === "error") throw new Error(command.errorMessage || t("failed"));
      toast.success(t("completed"));
      onOpenChange(false);
      await onQueued();
    } catch (error) {
      toast.error(error instanceof Error && error.message ? error.message : t("failed"));
    } finally {
      setSubmitting(false);
    }
  }, [create, deviceId, onOpenChange, onQueued, path, submitting, t]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <form className="contents" onSubmit={submit}>
          <DialogHeader>
            <DialogTitle>{t("title")}</DialogTitle>
            <DialogDescription>{t("description")}</DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-1.5">
              <label htmlFor={pathInputID} className="text-xs text-muted-foreground">{t("pathLabel")}</label>
              <Input
                id={pathInputID}
                autoFocus
                value={path}
                maxLength={4096}
                placeholder={t("pathPlaceholder")}
                disabled={submitting}
                onChange={(event) => setPath(event.target.value)}
              />
            </div>
            <label htmlFor={createInputID} className="flex items-start gap-2 text-sm">
              <Checkbox
                id={createInputID}
                checked={create}
                disabled={submitting}
                onCheckedChange={(checked) => setCreate(checked === true)}
              />
              <span>
                <span className="block">{t("createLabel")}</span>
                <span className="block text-xs leading-5 text-muted-foreground">{t("createDescription")}</span>
              </span>
            </label>
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" disabled={submitting} onClick={() => onOpenChange(false)}>
              {t("cancel")}
            </Button>
            <Button type="submit" disabled={!path.trim() || submitting}>
              {submitting ? <Spinner className="size-4" /> : null}
              {t("submit")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

export function WorkspaceRenameDialog({
  deviceId,
  target,
  onOpenChange,
  onRenamed,
}: {
  deviceId: string;
  target: { workspaceId: string; name: string } | null;
  onOpenChange: (open: boolean) => void;
  onRenamed: () => void | Promise<void>;
}) {
  const t = useTranslations("recent.projects.workspace");
  const [name, setName] = React.useState("");
  const [submitting, setSubmitting] = React.useState(false);
  const nameInputID = React.useId();

  React.useEffect(() => {
    setName(target?.name ?? "");
    if (!target) setSubmitting(false);
  }, [target]);

  const submit = React.useCallback(async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const normalizedName = name.trim();
    if (!target || !normalizedName || normalizedName === target.name || submitting) return;
    setSubmitting(true);
    try {
      const token = await resolveAccessToken();
      if (!token) throw new Error("missing access token");
      const queued = await renameAgentWorkspace(token, deviceId, target.workspaceId, normalizedName);
      const command = await waitForAgentCommand(token, queued.commandId);
      if (!command) throw new Error(t("timeout"));
      if (command.status === "error") throw new Error(command.errorMessage || t("renameFailed"));
      toast.success(t("renamed"));
      onOpenChange(false);
      await onRenamed();
    } catch (error) {
      toast.error(error instanceof Error && error.message ? error.message : t("renameFailed"));
    } finally {
      setSubmitting(false);
    }
  }, [deviceId, name, onOpenChange, onRenamed, submitting, t, target]);

  return (
    <Dialog open={Boolean(target)} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <form className="contents" onSubmit={submit}>
          <DialogHeader>
            <DialogTitle>{t("renameTitle")}</DialogTitle>
            <DialogDescription>{t("renameDescription")}</DialogDescription>
          </DialogHeader>
          <div className="space-y-1.5">
            <label htmlFor={nameInputID} className="text-xs text-muted-foreground">{t("nameLabel")}</label>
            <Input
              id={nameInputID}
              autoFocus
              value={name}
              maxLength={128}
              disabled={submitting}
              onChange={(event) => setName(event.target.value)}
            />
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" disabled={submitting} onClick={() => onOpenChange(false)}>
              {t("cancel")}
            </Button>
            <Button type="submit" disabled={!name.trim() || name.trim() === target?.name || submitting}>
              {submitting ? <Spinner className="size-4" /> : null}
              {t("save")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
