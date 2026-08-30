import { Suspense } from "react";

import { LoginRoute } from "@/app/(auth)/login/login-route";
import { AppLogo } from "@/shared/components/app-logo";
import { CustomBrandAttribution } from "@/shared/components/powered-by-deeix";

function LoginFallback() {
  return (
    <main className="flex min-h-screen items-center justify-center px-4 py-8 text-foreground" aria-busy="true">
      <div className="w-full max-w-[360px]">
        <AppLogo width={32} height={32} priority className="mx-auto h-9 w-auto" />
      </div>
      <CustomBrandAttribution className="fixed bottom-4 right-4" />
    </main>
  );
}

export default function Page() {
  return (
    <Suspense fallback={<LoginFallback />}>
      <LoginRoute />
    </Suspense>
  );
}
