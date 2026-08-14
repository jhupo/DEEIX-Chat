"use client";

import * as React from "react";
import { Check, RefreshCw, Sparkles } from "lucide-react";
import { useLocale, useTranslations } from "next-intl";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { CenteredEmptyState } from "@/components/ui/empty-state";
import { Skeleton } from "@/components/ui/skeleton";
import { useDevices } from "@/features/devices";
import { normalizeAgentPlugins } from "@/features/devices/model/agent-plugin-data";
import {
  getAgentProfileResource,
  listAgentRuntimeProfiles,
  refreshAgentProfileResource,
  type AgentResourceSnapshotDTO,
  type AgentRuntimeProfileDTO,
} from "@/shared/api/agent-gateway";
import { useAuthSession } from "@/shared/auth/auth-session-context";
import { cn } from "@/lib/utils";

const REFRESH_TIMEOUT_MS = 35_000;
const REFRESH_POLL_MS = 750;

function wait(duration: number): Promise<void> {
  return new Promise((resolve) => window.setTimeout(resolve, duration));
}

export function AgentPluginsPage() {
  const t = useTranslations("settings.agentPlugins");
  const locale = useLocale();
  const { accessToken } = useAuthSession();
  const { defaultDevice, defaultWorkspace } = useDevices();
  const [profile, setProfile] = React.useState<AgentRuntimeProfileDTO | null>(null);
  const [snapshot, setSnapshot] = React.useState<AgentResourceSnapshotDTO | null>(null);
  const [loading, setLoading] = React.useState(true);
  const [message, setMessage] = React.useState("");
  const requestVersionRef = React.useRef(0);

  const load = React.useCallback(async (forceRefresh: boolean) => {
    const requestVersion = requestVersionRef.current + 1;
    requestVersionRef.current = requestVersion;
    const device = defaultDevice;
    if (!device) {
      setProfile(null);
      setSnapshot(null);
      setMessage(t("noDevice"));
      setLoading(false);
      return;
    }

    setLoading(true);
    setMessage("");
    try {
      const profiles = await listAgentRuntimeProfiles(accessToken, device.deviceId);
      if (requestVersionRef.current !== requestVersion) return;
      const selectedProfile = profiles.find((item) => item.profileId === defaultWorkspace?.profileId) ?? profiles[0] ?? null;
      setProfile(selectedProfile);
      if (!selectedProfile) {
        setSnapshot(null);
        setMessage(t("noProfile"));
        return;
      }
      if (!selectedProfile.manifest.resources.profile.includes("plugins")) {
        setSnapshot(null);
        setMessage(t("notSupported"));
        return;
      }

      let nextSnapshot: AgentResourceSnapshotDTO | null = null;
      if (!forceRefresh) {
        try {
          nextSnapshot = await getAgentProfileResource(accessToken, device.deviceId, selectedProfile.profileId, "plugins");
        } catch {
          // A newly connected runtime has no snapshot until its first refresh completes.
        }
      }

      if (!nextSnapshot && device.online) {
        const startedAt = Date.now();
        await refreshAgentProfileResource(accessToken, device.deviceId, selectedProfile.profileId, "plugins");
        while (requestVersionRef.current === requestVersion && Date.now() - startedAt < REFRESH_TIMEOUT_MS) {
          await wait(REFRESH_POLL_MS);
          try {
            const candidate = await getAgentProfileResource(accessToken, device.deviceId, selectedProfile.profileId, "plugins");
            if (new Date(candidate.refreshedAt).getTime() >= startedAt - 1000) {
              nextSnapshot = candidate;
              break;
            }
          } catch {
            // Continue polling while the device processes the refresh command.
          }
        }
      }

      if (requestVersionRef.current !== requestVersion) return;
      if (!nextSnapshot) {
        setMessage(t("loadFailed"));
        return;
      }
      setSnapshot(nextSnapshot);
    } catch {
      if (requestVersionRef.current === requestVersion) setMessage(t("loadFailed"));
    } finally {
      if (requestVersionRef.current === requestVersion) setLoading(false);
    }
  }, [accessToken, defaultDevice, defaultWorkspace?.profileId, t]);

  React.useEffect(() => {
    setSnapshot(null);
    void load(false);
    return () => {
      requestVersionRef.current += 1;
    };
  }, [load]);

  const plugins = React.useMemo(() => normalizeAgentPlugins(snapshot?.data), [snapshot?.data]);
  const refreshedAt = snapshot?.refreshedAt
    ? new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "short" }).format(new Date(snapshot.refreshedAt))
    : "";

  return (
    <div className="flex h-full min-h-0 w-full flex-1 flex-col overflow-hidden">
      <div className="mx-auto flex h-full min-h-0 w-full max-w-[912px] flex-1 flex-col px-3 pb-8 pt-6 md:pt-15">
        <header className="flex items-start justify-between gap-4 md:ml-13 md:w-[calc(100%-3.25rem)]">
          <div className="min-w-0">
            <h1 className="text-xl font-semibold text-foreground md:text-2xl">{t("title")}</h1>
            {defaultDevice ? (
              <p className="mt-1 truncate text-xs text-muted-foreground">
                {defaultDevice.name}{profile?.provider ? ` · ${profile.provider}` : ""}
              </p>
            ) : null}
          </div>
          <div className="flex shrink-0 items-center gap-2">
            {refreshedAt ? <span className="hidden text-xs text-muted-foreground sm:inline">{t("refreshedAt", { time: refreshedAt })}</span> : null}
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label={t("refresh")}
              title={t("refresh")}
              disabled={loading || !defaultDevice?.online}
              onClick={() => void load(true)}
            >
              <RefreshCw aria-hidden className={cn("size-4", loading && "animate-spin")} />
            </Button>
          </div>
        </header>

        <div className="mt-8 min-h-0 flex-1 overflow-y-auto md:ml-13 md:w-[calc(100%-3.25rem)]">
          {loading && !snapshot ? (
            <div className="grid gap-2 sm:grid-cols-2">
              {Array.from({ length: 6 }).map((_, index) => (
                <Skeleton key={index} className="h-24 rounded-lg" />
              ))}
            </div>
          ) : message ? (
            <CenteredEmptyState className="min-h-64" title={message} />
          ) : plugins.length === 0 ? (
            <CenteredEmptyState className="min-h-64" title={t("empty")} />
          ) : (
            <div className="grid gap-2 sm:grid-cols-2">
              {plugins.map((plugin) => (
                <div key={plugin.id} className="min-w-0 rounded-lg bg-muted/35 px-3 py-3">
                  <div className="flex min-w-0 items-start gap-2">
                    <div className="min-w-0 flex-1">
                      <h2 className="truncate text-sm font-medium text-foreground" title={plugin.name}>{plugin.name}</h2>
                      <p className="mt-0.5 truncate text-xs text-muted-foreground" title={plugin.marketplace || plugin.source}>
                        {plugin.marketplace || plugin.source}
                      </p>
                    </div>
                    <div className="flex shrink-0 flex-wrap justify-end gap-1">
                      {plugin.installed ? <Badge variant="secondary"><Check aria-hidden />{t("installed")}</Badge> : null}
                      {plugin.enabled ? <Badge variant="outline">{t("enabled")}</Badge> : null}
                      {plugin.featured ? <Badge variant="outline"><Sparkles aria-hidden />{t("featured")}</Badge> : null}
                    </div>
                  </div>
                  {plugin.keywords.length > 0 || plugin.version ? (
                    <div className="mt-3 flex min-w-0 flex-wrap gap-x-2 gap-y-1 text-[11px] text-muted-foreground">
                      {plugin.version ? <span>{t("version", { version: plugin.version })}</span> : null}
                      {plugin.keywords.slice(0, 4).map((keyword) => <span key={keyword}>#{keyword}</span>)}
                    </div>
                  ) : null}
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
