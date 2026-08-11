"use client";

import * as React from "react";
import { useRouter } from "next/navigation";
import { useTranslations } from "next-intl";
import { toast } from "sonner";

import { completeEmailRegistration, getLoginOptions, getLoginPageSettings, login, startEmailRegistration, verifyTwoFactorLogin } from "@/shared/api/auth";
import type { LoginOptionsData, LoginPageSettings } from "@/shared/api/auth.types";
import { isPasswordPolicyValid } from "@/shared/auth/account-policy";
import { normalizeAuthNextPath } from "@/shared/auth/local-path";
import { resolveAccessToken } from "@/shared/auth/resolve-access-token";
import { writeSessionSnapshot } from "@/shared/auth/session";
import { useLocalizedErrorMessage } from "@/i18n/use-localized-error";
import {
  DEFAULT_LOGIN_OPTIONS,
  DEFAULT_LOGIN_SETTINGS,
  isTwoFactorChallengeExpired,
  normalizeRegisterCode,
  normalizeTwoFactorInput,
  TWO_FACTOR_CHALLENGE_STORAGE_KEY,
  type LoginMode,
} from "@/features/auth/model/login-page";

type UseLoginPageInput = {
  nextPath: string;
};

const VERIFICATION_CODE_RESEND_COOLDOWN_MS = 60_000;

export function useLoginPage({ nextPath }: UseLoginPageInput) {
  const router = useRouter();
  const t = useTranslations("login");
  const resolveErrorMessage = useLocalizedErrorMessage();
  const [settings, setSettings] = React.useState<LoginPageSettings>(DEFAULT_LOGIN_SETTINGS);
  const [options, setOptions] = React.useState<LoginOptionsData>(DEFAULT_LOGIN_OPTIONS);
  const [email, setEmail] = React.useState("");
  const [password, setPassword] = React.useState("");
  const [loginTurnstileToken, setLoginTurnstileToken] = React.useState("");
  const [loginTurnstileResetSignal, setLoginTurnstileResetSignal] = React.useState(0);
  const [twoFactorChallengeToken, setTwoFactorChallengeToken] = React.useState("");
  const [twoFactorCode, setTwoFactorCode] = React.useState("");
  const [mode, setMode] = React.useState<LoginMode>("login");
  const [registerEmail, setRegisterEmail] = React.useState("");
  const [registerPassword, setRegisterPassword] = React.useState("");
  const [registerCode, setRegisterCode] = React.useState("");
  const [registerDebugCode, setRegisterDebugCode] = React.useState("");
  const [registerTurnstileToken, setRegisterTurnstileToken] = React.useState("");
  const [registerTurnstileResetSignal, setRegisterTurnstileResetSignal] = React.useState(0);
  const [codeSent, setCodeSent] = React.useState(false);
  const [configReady, setConfigReady] = React.useState(false);
  const [submitting, setSubmitting] = React.useState(false);
  const [sendingCode, setSendingCode] = React.useState(false);
  const [registerCodeResendAt, setRegisterCodeResendAt] = React.useState(0);
  const [cooldownNow, setCooldownNow] = React.useState(() => Date.now());
  const registerCodeCooldownSeconds = Math.max(0, Math.ceil((registerCodeResendAt - cooldownNow) / 1000));

  const fallbackNextPath = normalizeAuthNextPath(settings.defaultNextPath);
  const resolvedNextPath = normalizeAuthNextPath(nextPath, fallbackNextPath);
  const emailRegistrationEnabled = options.emailRegistrationEnabled;
  const emailVerificationEnabled = options.emailVerificationEnabled;
  const turnstileSiteKey = options.turnstileSiteKey?.trim() ?? "";
  const turnstileRequired = options.turnstileEnabled && Boolean(turnstileSiteKey);

  React.useEffect(() => {
    if (registerCodeCooldownSeconds === 0) return undefined;
    const timer = window.setInterval(() => setCooldownNow(Date.now()), 1000);
    return () => window.clearInterval(timer);
  }, [registerCodeCooldownSeconds]);

  React.useEffect(() => {
    let mounted = true;
    void resolveAccessToken().then((token) => {
      if (mounted && token) router.replace(resolvedNextPath);
    }).catch(() => undefined);
    return () => {
      mounted = false;
    };
  }, [resolvedNextPath, router]);

  React.useEffect(() => {
    const challenge = window.sessionStorage.getItem(TWO_FACTOR_CHALLENGE_STORAGE_KEY);
    if (challenge) {
      window.sessionStorage.removeItem(TWO_FACTOR_CHALLENGE_STORAGE_KEY);
      setTwoFactorChallengeToken(challenge);
    }
  }, []);

  React.useEffect(() => {
    let cancelled = false;
    void Promise.all([getLoginPageSettings(), getLoginOptions()])
      .then(([pageSettings, loginOptions]) => {
        if (!cancelled) {
          setSettings(pageSettings);
          setOptions(loginOptions);
        }
      })
      .catch(() => undefined)
      .finally(() => {
        if (!cancelled) setConfigReady(true);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  React.useEffect(() => {
    if (mode === "register" && !emailRegistrationEnabled) setMode("login");
  }, [emailRegistrationEnabled, mode]);

  const completeAuth = React.useCallback((accessToken?: string, sessionID?: string) => {
    if (!accessToken || !sessionID) {
      throw new Error(t("loginRetry"));
    }
    writeSessionSnapshot({ accessToken, sessionID });
    router.replace(resolvedNextPath);
  }, [resolvedNextPath, router, t]);

  const resetRegisterTurnstile = React.useCallback(() => {
    setRegisterTurnstileToken("");
    setRegisterTurnstileResetSignal((current) => current + 1);
  }, []);

  const resetLoginTurnstile = React.useCallback(() => {
    setLoginTurnstileToken("");
    setLoginTurnstileResetSignal((current) => current + 1);
  }, []);

  const onLoginSubmit = React.useCallback(async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (submitting) return;
    if (!twoFactorChallengeToken && turnstileRequired && !loginTurnstileToken) {
      toast.error(t("toasts.turnstileRequired"));
      return;
    }
    setSubmitting(true);
    try {
      const formData = new FormData(event.currentTarget);
      const result = twoFactorChallengeToken
        ? await verifyTwoFactorLogin(twoFactorChallengeToken, normalizeTwoFactorInput(String(formData.get("otp") ?? twoFactorCode)))
        : await login(String(formData.get("email") ?? email).trim(), String(formData.get("password") ?? password), turnstileRequired ? loginTurnstileToken : undefined);
      if (result.twoFactorRequired) {
        setTwoFactorChallengeToken(result.twoFactorChallengeToken ?? "");
        setTwoFactorCode("");
        return;
      }
      if (!result.accessToken || !result.sessionID) {
        toast.error(t("toasts.loginFailed"));
        return;
      }
      completeAuth(result.accessToken, result.sessionID);
    } catch (error) {
      if (isTwoFactorChallengeExpired(error)) {
        setTwoFactorChallengeToken("");
        setTwoFactorCode("");
        toast.error(t("toasts.challengeExpired"));
      } else {
        toast.error(resolveErrorMessage(error, t("toasts.loginRetry")));
      }
    } finally {
      if (!twoFactorChallengeToken && turnstileRequired) resetLoginTurnstile();
      setSubmitting(false);
    }
  }, [completeAuth, email, turnstileRequired, loginTurnstileToken, password, resetLoginTurnstile, resolveErrorMessage, submitting, t, twoFactorChallengeToken, twoFactorCode]);

  const requestRegisterCode = React.useCallback(async () => {
    if (!emailVerificationEnabled || sendingCode || registerCodeCooldownSeconds > 0) return;
    if (turnstileRequired && !registerTurnstileToken) {
      toast.error(t("toasts.turnstileRequired"));
      return;
    }
    setSendingCode(true);
    try {
      const result = await startEmailRegistration(registerEmail, turnstileRequired ? registerTurnstileToken : undefined);
      setCodeSent(result.sent);
      setRegisterDebugCode(result.debugCode ?? "");
      if (result.sent) {
        const now = Date.now();
        setCooldownNow(now);
        setRegisterCodeResendAt(now + VERIFICATION_CODE_RESEND_COOLDOWN_MS);
      }
    } catch (error) {
      toast.error(resolveErrorMessage(error, t("toasts.codeSendFailed")));
    } finally {
      if (turnstileRequired && registerTurnstileToken) resetRegisterTurnstile();
      setSendingCode(false);
    }
  }, [emailVerificationEnabled, registerCodeCooldownSeconds, registerEmail, turnstileRequired, registerTurnstileToken, resetRegisterTurnstile, resolveErrorMessage, sendingCode, t]);

  const onRegisterSubmit = React.useCallback(async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (submitting) return;
    if (!isPasswordPolicyValid(registerPassword)) {
      toast.error(t("toasts.passwordInvalid"));
      return;
    }
    if (turnstileRequired && !registerTurnstileToken) {
      toast.error(t("toasts.turnstileRequired"));
      return;
    }
    setSubmitting(true);
    try {
      const result = await completeEmailRegistration(registerEmail, registerPassword, emailVerificationEnabled ? registerCode : "", turnstileRequired ? registerTurnstileToken : undefined);
      if (!result.accessToken || !result.sessionID) {
        toast.error(t("toasts.registerFailed"));
        return;
      }
      completeAuth(result.accessToken, result.sessionID);
    } catch (error) {
      toast.error(resolveErrorMessage(error, t("toasts.registerFailed")));
    } finally {
      if (turnstileRequired && registerTurnstileToken) resetRegisterTurnstile();
      setSubmitting(false);
    }
  }, [completeAuth, emailVerificationEnabled, registerCode, registerEmail, registerPassword, turnstileRequired, registerTurnstileToken, resetRegisterTurnstile, resolveErrorMessage, submitting, t]);

  const cancelTwoFactorChallenge = React.useCallback(() => {
    setTwoFactorChallengeToken("");
    setTwoFactorCode("");
  }, []);

  const toggleLoginMode = React.useCallback(() => {
    setMode((current) => current === "login" ? "register" : "login");
  }, []);

  return {
    cancelTwoFactorChallenge,
    codeSent,
    configReady,
    emailRegistrationEnabled,
    emailVerificationEnabled,
    email,
    turnstileRequired,
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
    setRegisterCode: (value: string) => setRegisterCode(normalizeRegisterCode(value)),
    setRegisterPassword,
    setRegisterTurnstileToken,
    turnstileSiteKey,
    submitting,
    toggleLoginMode,
    twoFactorChallengeToken,
    twoFactorCode,
    setTwoFactorCode,
    updateRegisterEmail: setRegisterEmail,
  };
}
