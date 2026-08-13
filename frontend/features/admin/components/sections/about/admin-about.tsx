"use client";

import { useCallback, useEffect, useRef, useState, useSyncExternalStore } from "react";
import { CircleArrowUp, RefreshCw } from "lucide-react";
import { useTranslations } from "next-intl";

import packageMeta from "@/package.json";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { AdminUpdateTooltipContent } from "@/features/admin/components/admin-update-tooltip-content";
import {
  checkAdminUpdate,
  getAdminUpdateJob,
  getAdminUpdateStatus,
  installAdminUpdate,
  restartAdminUpdate,
  type AdminUpdateJob,
  type AdminUpdateStatus,
} from "@/features/admin/api/update";
import {
  formatReleaseVersion,
  getCachedLatestReleaseSnapshot,
  getServerLatestReleaseSnapshot,
  resolveAvailableRelease,
  subscribeLatestReleaseChange,
  writeCachedLatestRelease,
} from "@/features/admin/model/update-check";
import { AboutSettingsContent } from "@/shared/components/about-settings-content";
import { useAuthSession } from "@/shared/auth/auth-session-context";
import { useDialogSnapshot } from "@/shared/hooks/use-dialog-snapshot";
import { createIdempotencyKey } from "@/shared/lib/idempotency-key";
import { cn } from "@/lib/utils";

type DialogState = "checking" | "current" | "available" | "confirming" | "job" | "failed";

const terminalStatuses = new Set<AdminUpdateJob["status"]>([
  "succeeded",
  "failed",
  "outcome_unknown",
]);

function dialogForStatus(status: AdminUpdateStatus): DialogState {
  if (relevantJob(status)) {
    return "job";
  }
  return status.updateAvailable ? "available" : "current";
}

function relevantJob(status: AdminUpdateStatus): AdminUpdateJob | undefined {
  const job = status.job;
  if (
    !job ||
    (terminalStatuses.has(job.status) &&
      (job.version !== status.candidate?.version || job.version === status.installedVersion))
  ) {
    return undefined;
  }
  return job;
}

function cacheCandidate(status: AdminUpdateStatus) {
  if (status.candidate) {
    writeCachedLatestRelease({
      version: status.candidate.version,
      url: status.candidate.releaseURL,
    });
  }
}

function AdminAboutVersionBadge({ available }: { available: boolean }) {
  const t = useTranslations("adminUsers.aboutPage");

  return (
    <span className="inline-flex items-center gap-1.5">
      <span>{formatReleaseVersion(packageMeta.version)}</span>
      {available ? (
        <CircleArrowUp
          className="size-3.5 text-rose-500"
          aria-label={t("updateAvailableIndicator")}
        />
      ) : null}
    </span>
  );
}

function AdminUpdateCheck() {
  const t = useTranslations("adminUsers.aboutPage");
  const { accessToken, user } = useAuthSession();
  const [status, setStatus] = useState<AdminUpdateStatus | null>(null);
  const [dialog, setDialog] = useState<DialogState | null>(null);
  const [checking, setChecking] = useState(false);
  const [installing, setInstalling] = useState(false);
  const [installError, setInstallError] = useState(false);
  const [restartError, setRestartError] = useState(false);
  const [reconnecting, setReconnecting] = useState(false);
  const [key, setKey] = useState("");
  const completedJobID = useRef("");
  const superadmin = user?.role === "superadmin";
  const job = status ? relevantJob(status) : undefined;
  const jobID = job?.id ?? "";
  const jobStatus = job?.status;
  const candidate = status?.candidate;

  const loadStatus = useCallback(
    async (open: boolean) => {
      const next = await getAdminUpdateStatus(accessToken);
      setStatus(next);
      cacheCandidate(next);
      if (open) setDialog(dialogForStatus(next));
      return next;
    },
    [accessToken],
  );

  useEffect(() => {
    if (!superadmin) return;
    void loadStatus(false).catch(() => undefined);
  }, [loadStatus, superadmin]);

  useEffect(() => {
    if (!jobID || !jobStatus || terminalStatuses.has(jobStatus)) return;

    const timer = window.setInterval(() => {
      void getAdminUpdateJob(accessToken, jobID)
        .then((job) => {
          setReconnecting(false);
          setStatus((current) => (current ? { ...current, job } : current));
          if (terminalStatuses.has(job.status) && completedJobID.current !== job.id) {
            completedJobID.current = job.id;
            void loadStatus(false).catch(() => setReconnecting(true));
          }
        })
        .catch(() => setReconnecting(true));
    }, 2000);

    return () => window.clearInterval(timer);
  }, [accessToken, jobID, jobStatus, loadStatus]);

  const state = useDialogSnapshot(dialog);

  if (!superadmin) return null;

  const check = async () => {
    if (checking) return;
    setChecking(true);
    setDialog("checking");
    try {
      const next = await checkAdminUpdate(accessToken);
      setStatus(next);
      cacheCandidate(next);
      setDialog(dialogForStatus(next));
    } catch {
      setDialog("failed");
    } finally {
      setChecking(false);
    }
  };

  const beginInstall = () => {
    setKey("");
    setInstallError(false);
    setDialog("confirming");
  };

  const startInstall = async () => {
    if (!candidate || installing) return;

    const attemptKey = key || createIdempotencyKey();
    setKey(attemptKey);
    setInstallError(false);
    setInstalling(true);
    try {
      const job = await installAdminUpdate(accessToken, attemptKey, {
        version: candidate.version,
        manifestDigest: candidate.manifestDigest,
        confirmation: `install ${candidate.version} ${candidate.manifestDigest}`,
      });
      setStatus((current) => (current ? { ...current, job } : current));
      setDialog("job");
    } catch {
      setInstallError(true);
      setDialog("confirming");
    } finally {
      setInstalling(false);
    }
  };

  const restart = async () => {
    if (!candidate || reconnecting) return;
    setRestartError(false);
    setReconnecting(true);
    try {
      await restartAdminUpdate(accessToken);
      for (let attempt = 0; attempt < 60; attempt += 1) {
        await new Promise((resolve) => window.setTimeout(resolve, 2000));
        try {
          const next = await getAdminUpdateStatus(accessToken);
          if (next.installedVersion === candidate.version) {
            window.location.reload();
            return;
          }
        } catch {
          // The application is expected to be briefly unavailable while restarting.
        }
      }
      setRestartError(true);
    } catch {
      setRestartError(true);
    } finally {
      setReconnecting(false);
    }
  };

  const current = status?.installedVersion || packageMeta.version;
  const canInstall = Boolean(
    candidate &&
      status?.updateAvailable &&
      (!jobStatus || terminalStatuses.has(jobStatus)) &&
      !installing,
  );
  const statusLabel = jobStatus ? t(`updateDialog.states.${jobStatus}`) : t("updateDialog.ready");
  const retryableJob = jobStatus === "failed" || jobStatus === "outcome_unknown";

  return (
    <>
      <button
        type="button"
        className="inline-flex items-center gap-1 text-xs text-muted-foreground/80 transition-colors hover:text-foreground disabled:cursor-wait disabled:opacity-70"
        onClick={() => void check()}
        disabled={checking}
      >
        <RefreshCw className={cn("size-3", checking && "animate-spin")} />
        <span>{checking ? t("checkingUpdate") : t("checkUpdate")}</span>
      </button>
      <Dialog open={dialog !== null} onOpenChange={(open) => !open && setDialog(null)}>
        <DialogContent className="flex max-h-[min(86vh,760px)] flex-col gap-0 overflow-hidden p-0 sm:max-w-[420px]">
          <DialogHeader className="shrink-0 px-4 py-4">
            <DialogTitle>
              {state === "failed"
                ? t("updateDialog.failedTitle")
                : state === "checking"
                  ? t("updateDialog.checkingTitle")
                : state === "job"
                  ? t("updateDialog.jobTitle")
                  : state === "confirming"
                    ? t("updateDialog.confirmTitle")
                    : state === "available"
                      ? t("updateDialog.availableTitle")
                      : t("updateDialog.currentTitle")}
            </DialogTitle>
            <DialogDescription>
              {state === "failed"
                ? t("updateDialog.failedDescription")
                : state === "checking"
                  ? t("updateDialog.checkingDescription")
                  : state === "job"
                    ? t("updateDialog.jobDescription", { version: candidate?.version ?? job?.version ?? "" })
                    : state === "confirming"
                      ? t("updateDialog.confirmDescription", { version: candidate?.version ?? "" })
                      : state === "available"
                        ? t("updateDialog.availableDescription", { current, latest: candidate?.version ?? "" })
                        : t("updateDialog.currentDescription", { current })}
            </DialogDescription>
          </DialogHeader>
          <div className="min-h-0 flex-1 overflow-y-auto px-4 py-2" aria-live="polite">
            {state === "checking" ? (
              <div className="flex min-h-16 items-center justify-center">
                <RefreshCw className="size-5 animate-spin text-muted-foreground" aria-hidden="true" />
              </div>
            ) : null}
            {installError ? (
              <p className="mb-3 text-xs text-destructive">{t("updateDialog.installFailed")}</p>
            ) : null}
            {restartError ? (
              <p className="mb-3 text-xs text-destructive">{t("updateDialog.restartFailed")}</p>
            ) : null}
            {candidate ? (
              <div className="rounded-md bg-muted/50 px-3 py-2 text-xs">
                <div className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2">
                  <span className="text-muted-foreground">{t("updateDialog.currentVersion")}</span>
                  <span>{current}</span>
                  <span className="text-muted-foreground">{t("updateDialog.latestVersion")}</span>
                  <span>{candidate.version}</span>
                  {state === "job" ? (
                    <>
                      <span className="text-muted-foreground">{t("updateDialog.status")}</span>
                      <span>{reconnecting ? t("updateDialog.reconnecting") : statusLabel}</span>
                    </>
                  ) : null}
                </div>
              </div>
            ) : null}
          </div>
          <DialogFooter className="shrink-0 px-4 py-3">
            <DialogClose asChild>
              <Button type="button" variant="ghost">
                {t("updateDialog.close")}
              </Button>
            </DialogClose>
            {state === "failed" ? (
              <Button type="button" onClick={() => void check()}>
                {t("updateDialog.retry")}
              </Button>
            ) : null}
            {candidate ? (
              <Button asChild type="button" variant="outline">
                <a href={candidate.releaseURL} target="_blank" rel="noopener noreferrer">
                  {t("updateDialog.openRelease")}
                </a>
              </Button>
            ) : null}
            {state === "available" ? (
              <Button type="button" disabled={!canInstall} onClick={beginInstall}>
                {t("updateDialog.install")}
              </Button>
            ) : null}
            {state === "confirming" ? (
              <Button type="button" disabled={!canInstall} onClick={() => void startInstall()}>
                {installing ? t("updateDialog.installing") : t("updateDialog.confirmAction")}
              </Button>
            ) : null}
            {retryableJob ? (
              <Button type="button" disabled={!canInstall} onClick={beginInstall}>
                {t("updateDialog.retryInstall")}
              </Button>
            ) : null}
            {jobStatus === "succeeded" ? (
              <Button type="button" disabled={reconnecting} onClick={() => void restart()}>
                {t("updateDialog.reload")}
              </Button>
            ) : null}
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

export function AdminAboutPage() {
  const t = useTranslations("adminUsers.aboutPage");
  const cached = useSyncExternalStore(
    subscribeLatestReleaseChange,
    getCachedLatestReleaseSnapshot,
    getServerLatestReleaseSnapshot,
  );
  const updateRelease = resolveAvailableRelease(packageMeta.version, cached);

  return (
    <AboutSettingsContent
      title={t("title")}
      description={t("description")}
      consoleLabel={t("adminConsole")}
      versionBadgeContent={<AdminAboutVersionBadge available={Boolean(updateRelease)} />}
      versionBadgeTooltip={<AdminUpdateTooltipContent updateRelease={updateRelease} />}
      versionActions={<AdminUpdateCheck />}
      labels={{
        details: t("details"),
        official: t("official"),
        website: t("website"),
        repository: t("repository"),
        social: t("social"),
        blog: t("blog"),
        contact: t("contact"),
        copyright: t("copyright"),
        license: t("license"),
      }}
    />
  );
}
