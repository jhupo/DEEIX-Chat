"use client";

import * as React from "react";
import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { SpinnerLabel } from "@/components/ui/spinner";
import { PASSWORD_MIN_LENGTH, isPasswordPolicyValid } from "@/shared/auth/account-policy";

export function ChangePasswordDialog({
  open,
  onOpenChange,
  pending,
  onSubmit,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  pending: boolean;
  onSubmit: (payload: { currentPassword: string; newPassword: string }) => Promise<void>;
}) {
  const t = useTranslations("settings.accountPage.securityDialog.password");
  const common = useTranslations("settings.accountPage.securityDialog.common");
  const [currentPassword, setCurrentPassword] = React.useState("");
  const [newPassword, setNewPassword] = React.useState("");

  React.useEffect(() => {
    if (!open) {
      setCurrentPassword("");
      setNewPassword("");
    }
  }, [open]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("title.change")}</DialogTitle>
          <DialogDescription>{t("description.change")}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-1">
            <p className="text-xs text-muted-foreground">{t("labels.currentPassword")}</p>
            <Input id="current-password" type="password" autoComplete="current-password" value={currentPassword} onChange={(event) => setCurrentPassword(event.target.value)} disabled={pending} />
          </div>
          <div className="space-y-1">
            <p className="text-xs text-muted-foreground">{t("labels.newPassword")}</p>
            <Input id="new-password" type="password" autoComplete="new-password" placeholder={t("placeholders.password")} value={newPassword} onChange={(event) => setNewPassword(event.target.value)} disabled={pending} minLength={PASSWORD_MIN_LENGTH} />
          </div>
        </div>
        <DialogFooter>
          <Button variant="ghost" disabled={pending} onClick={() => onOpenChange(false)}>{common("cancel")}</Button>
          <Button type="button" disabled={pending || !currentPassword || !isPasswordPolicyValid(newPassword)} onClick={() => void onSubmit({ currentPassword, newPassword })}>
            {pending ? <SpinnerLabel>{common("saving")}</SpinnerLabel> : common("save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
