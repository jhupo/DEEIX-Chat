"use client";

import { useTranslations } from "next-intl";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { SpinnerLabel } from "@/components/ui/spinner";
import { PASSWORD_MIN_LENGTH } from "@/shared/auth/account-policy";
import { useLoginPage } from "@/features/auth/hooks/use-auth-login-page";
import { AppLogo } from "@/shared/components/app-logo";
import { CustomBrandAttribution } from "@/shared/components/powered-by-deeix";
import { TurnstileWidget } from "@/features/auth/components/turnstile-widget";
import { cn } from "@/lib/utils";

type LoginPageProps = {
  nextPath: string;
};

function LoginBrandMark() {
  return <AppLogo width={32} height={32} priority className="mx-auto h-9 w-auto" />;
}

export function LoginPage({ nextPath }: LoginPageProps) {
  const t = useTranslations("login");
  const loginPage = useLoginPage({ nextPath });
  const {
    cancelTwoFactorChallenge,
    codeSent,
    configReady,
    email,
    emailRegistrationEnabled,
    emailVerificationEnabled,
    loginTurnstileResetSignal,
    loginTurnstileToken,
    mode,
    onLoginSubmit,
    onRegisterSubmit,
    password,
    registerCode,
    registerCodeCooldownSeconds,
    registerDebugCode,
    registerEmail,
    registerPassword,
    registerTurnstileResetSignal,
    registerTurnstileToken,
    requestRegisterCode,
    sendingCode,
    setEmail,
    setLoginTurnstileToken,
    setPassword,
    setRegisterCode,
    setRegisterPassword,
    setRegisterTurnstileToken,
    submitting,
    toggleLoginMode,
    twoFactorChallengeToken,
    twoFactorCode,
    setTwoFactorCode,
    turnstileRequired,
    turnstileSiteKey,
    updateRegisterEmail,
  } = loginPage;

  return (
    <main className="flex min-h-screen items-center justify-center px-4 py-8 text-foreground" aria-busy={!configReady}>
      <div className="w-full max-w-[360px]">
        <LoginBrandMark />
        <div
          aria-hidden={!configReady}
          className={cn(
            "grid transition-[grid-template-rows,opacity] duration-200 ease-out",
            configReady ? "grid-rows-[1fr] opacity-100" : "pointer-events-none grid-rows-[0fr] opacity-0",
          )}
        >
          {configReady ? (
            <div className="min-h-0 overflow-hidden px-2">
              {mode === "login" && twoFactorChallengeToken ? (
                <>
                  <form className="mt-7 space-y-4" onSubmit={onLoginSubmit}>
                    <div className="space-y-2">
                      <label className="text-sm font-medium leading-none text-foreground" htmlFor="otp">
                        {t("twoFactorCode")}
                      </label>
                      <Input
                        id="otp"
                        name="otp"
                        type="text"
                        autoComplete="one-time-code"
                        className="h-9 border-input/50"
                        placeholder={t("twoFactorPlaceholder")}
                        value={twoFactorCode}
                        onChange={(event) => setTwoFactorCode(event.target.value)}
                        required
                      />
                    </div>
                    <Button className="mt-1 h-9 w-full rounded-md bg-foreground text-sm font-semibold text-background shadow-none hover:bg-foreground/90" type="submit" disabled={submitting}>
                      {submitting ? <SpinnerLabel>{t("signingIn")}</SpinnerLabel> : t("verifyAndSignIn")}
                    </Button>
                  </form>
                  <Button type="button" variant="ghost" className="mt-2 h-9 w-full text-xs text-muted-foreground shadow-none" onClick={cancelTwoFactorChallenge}>
                    {t("backToPasswordLogin")}
                  </Button>
                </>
              ) : null}

              {mode === "login" && !twoFactorChallengeToken ? (
                <form className="mt-7 space-y-4" onSubmit={onLoginSubmit}>
                  <div className="space-y-2">
                    <label className="text-sm font-medium leading-none text-foreground" htmlFor="email">{t("email")}</label>
                    <Input id="email" name="email" type="email" autoComplete="username" className="h-9 border-input/50" placeholder={t("email")} value={email} onChange={(event) => setEmail(event.target.value)} required />
                  </div>
                  <div className="space-y-2">
                    <label className="text-sm font-medium leading-none text-foreground" htmlFor="password">{t("password")}</label>
                    <Input id="password" name="password" type="password" autoComplete="current-password" className="h-9 border-input/50" placeholder={t("password")} value={password} onChange={(event) => setPassword(event.target.value)} required />
                  </div>
                  {turnstileRequired ? <TurnstileWidget siteKey={turnstileSiteKey} resetSignal={loginTurnstileResetSignal} onTokenChange={setLoginTurnstileToken} /> : null}
                  <Button className="mt-2 h-9 w-full rounded-md bg-foreground text-sm font-semibold text-background shadow-none hover:bg-foreground/90" type="submit" disabled={submitting || (turnstileRequired && !loginTurnstileToken)}>
                    {submitting ? <SpinnerLabel>{t("signingIn")}</SpinnerLabel> : t("signIn")}
                  </Button>
                </form>
              ) : null}

              {mode === "register" && emailRegistrationEnabled ? (
                <form className="mt-7 space-y-4" onSubmit={onRegisterSubmit}>
                  <div className="space-y-2">
                    <label className="text-sm font-medium leading-none text-foreground" htmlFor="register-email">{t("email")}</label>
                    <Input id="register-email" type="email" autoComplete="email" className="h-9 border-input/50" placeholder={t("email")} value={registerEmail} onChange={(event) => updateRegisterEmail(event.target.value)} required />
                  </div>
                  <div className="space-y-2">
                    <label className="text-sm font-medium leading-none text-foreground" htmlFor="register-password">{t("password")}</label>
                    <Input id="register-password" type="password" autoComplete="new-password" className="h-9 border-input/50" placeholder={t("newPasswordPlaceholder")} value={registerPassword} onChange={(event) => setRegisterPassword(event.target.value)} minLength={PASSWORD_MIN_LENGTH} required />
                  </div>
                  {turnstileRequired ? <TurnstileWidget siteKey={turnstileSiteKey} resetSignal={registerTurnstileResetSignal} onTokenChange={setRegisterTurnstileToken} /> : null}
                  {emailVerificationEnabled ? (
                    <div className="space-y-2">
                      <label className="text-sm font-medium leading-none text-foreground" htmlFor="register-code">{t("verificationCode")}</label>
                      <div className="flex gap-2">
                        <Input id="register-code" inputMode="numeric" autoComplete="one-time-code" className="h-9 border-input/50" placeholder={t("verificationCodePlaceholder")} value={registerCode} onChange={(event) => setRegisterCode(event.target.value)} required />
                        <Button type="button" variant="secondary" className="h-9 min-w-[4.5rem] shrink-0 rounded-md border-0 bg-muted px-3 text-sm font-semibold text-foreground shadow-none hover:bg-muted/80" disabled={sendingCode || registerCodeCooldownSeconds > 0 || !registerEmail.trim() || (turnstileRequired && !registerTurnstileToken)} onClick={() => void requestRegisterCode()}>
                          {sendingCode ? <SpinnerLabel>{t("sending")}</SpinnerLabel> : registerCodeCooldownSeconds > 0 ? t("resendIn", { seconds: registerCodeCooldownSeconds }) : codeSent ? t("resend") : t("send")}
                        </Button>
                      </div>
                    </div>
                  ) : null}
                  {registerDebugCode ? <p className="text-xs font-medium text-muted-foreground">{t("debugCode", { code: registerDebugCode })}</p> : null}
                  <Button className="mt-1 h-9 w-full rounded-md bg-foreground text-sm font-semibold text-background shadow-none hover:bg-foreground/90" type="submit" disabled={submitting || (emailVerificationEnabled && registerCode.length !== 6) || (turnstileRequired && !registerTurnstileToken)}>
                    {submitting ? <SpinnerLabel>{t("registering")}</SpinnerLabel> : t("register")}
                  </Button>
                </form>
              ) : null}

              {emailRegistrationEnabled ? (
                <div className="mt-6 text-center text-sm font-normal leading-5 text-muted-foreground">
                  {mode === "register" ? t("alreadyHaveAccount") : t("noAccount")} {" "}
                  <button type="button" className="font-semibold text-foreground underline-offset-4 hover:underline focus-visible:underline focus-visible:outline-none" onClick={toggleLoginMode}>
                    {mode === "register" ? t("signIn") : t("register")}
                  </button>
                </div>
              ) : null}
            </div>
          ) : null}
        </div>
      </div>
      <CustomBrandAttribution className="fixed bottom-4 right-4" />
    </main>
  );
}
