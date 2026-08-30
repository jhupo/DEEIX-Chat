"use client";

import { useRouter } from "next/navigation";
import * as React from "react";

import { SettingsSubscription } from "@/features/settings/components/sections/subscription/settings-subscription";
import { useAuthSession } from "@/shared/auth/auth-session-context";

export default function SettingsSubscriptionPage() {
  const router = useRouter();
  const { user, userStatus } = useAuthSession();
  const relayAvailable = userStatus === "ready" && user?.authProvider === "relay";

  React.useEffect(() => {
    if (userStatus === "ready" && !relayAvailable) {
      router.replace("/setting/general");
    }
  }, [relayAvailable, router, userStatus]);

  return relayAvailable ? <SettingsSubscription /> : null;
}
