"use client";

import * as React from "react";
import { Apple, Monitor, Terminal } from "lucide-react";
import { useTranslations } from "next-intl";

import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { CopyActionButton } from "@/shared/components/copy-action";

type Platform = "windows" | "macos" | "linux";

export function AccountAddDeviceDialog({
  open,
  onOpenChange,
  publicUserID,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  publicUserID: string;
}) {
  const t = useTranslations("settings.accountPage.device.install");
  const [origin, setOrigin] = React.useState("");
  React.useEffect(() => setOrigin(window.location.origin), []);

  const command = React.useCallback((platform: Platform) => {
    if (!origin || !publicUserID) return "";
    if (platform === "windows") {
      return `& ([scriptblock]::Create((irm '${origin}/agent/install.ps1'))) -Server '${origin}' -User '${publicUserID}'`;
    }
    return `curl -fsSL '${origin}/agent/install.sh' | sh -s -- --server '${origin}' --user '${publicUserID}'`;
  }, [origin, publicUserID]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[620px]">
        <DialogHeader>
          <DialogTitle>{t("title")}</DialogTitle>
          <DialogDescription>{t("description")}</DialogDescription>
        </DialogHeader>
        <Tabs defaultValue="windows">
          <TabsList className="grid w-full grid-cols-3">
            <TabsTrigger value="windows"><Monitor />Windows</TabsTrigger>
            <TabsTrigger value="macos"><Apple />macOS</TabsTrigger>
            <TabsTrigger value="linux"><Terminal />Linux</TabsTrigger>
          </TabsList>
          {(["windows", "macos", "linux"] as const).map((platform) => (
            <TabsContent key={platform} value={platform} className="mt-3">
              <div className="flex items-start gap-2 rounded-md border bg-muted/25 p-3">
                <code className="min-w-0 flex-1 break-all text-xs leading-5">{command(platform)}</code>
                <CopyActionButton
                  type="button"
                  variant="outline"
                  size="icon"
                  className="size-8 shrink-0"
                  value={command(platform)}
                  disabled={!command(platform)}
                  aria-label={t("copy")}
                  messages={{ copied: t("copied"), failed: t("copyFailed") }}
                />
              </div>
            </TabsContent>
          ))}
        </Tabs>
      </DialogContent>
    </Dialog>
  );
}
