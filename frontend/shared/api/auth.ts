import { apiRequest, pathParam } from "@/shared/api/http-client";
import { authedRequest } from "@/shared/api/authed-client";
import type {
  ActiveSessionDTO,
  ActiveSessionListData,
  ChangePasswordData,
  ChangePasswordPayload,
  EmailRegistrationStartData,
  LoginData,
  LoginOptionsData,
  LoginPageSettings,
  LogoutData,
  MeData,
  PatchMePayload,
  UpdateCurrentSessionLocationPayload,
  UserDTO,
} from "@/shared/api/auth.types";

export async function login(email: string, password: string, turnstileToken?: string): Promise<LoginData> {
  return apiRequest<LoginData>("/api/v1/auth/login", {
    method: "POST",
    body: { email, password, turnstileToken },
  });
}

export async function verifyTwoFactorLogin(challengeToken: string, code: string): Promise<LoginData> {
  return apiRequest<LoginData>("/api/v1/auth/login/2fa", {
    method: "POST",
    body: { challengeToken, code },
  });
}

export async function startEmailRegistration(email: string, turnstileToken?: string): Promise<EmailRegistrationStartData> {
  return apiRequest<EmailRegistrationStartData>("/api/v1/auth/register/email/start", {
    method: "POST",
    body: { email, turnstileToken },
  });
}

export async function completeEmailRegistration(email: string, password: string, code: string, turnstileToken?: string): Promise<LoginData> {
  return apiRequest<LoginData>("/api/v1/auth/register/email/complete", {
    method: "POST",
    body: { email, password, code, turnstileToken },
  });
}

export async function changePassword(accessToken: string, payload: ChangePasswordPayload): Promise<ChangePasswordData> {
  return authedRequest<ChangePasswordData>(
    "/api/v1/me/password",
    { method: "PUT", accessToken, body: payload },
    false,
  );
}

export async function getLoginPageSettings(): Promise<LoginPageSettings> {
  return apiRequest<LoginPageSettings>("/api/v1/settings/login-page");
}

export async function getLoginOptions(): Promise<LoginOptionsData> {
  return apiRequest<LoginOptionsData>("/api/v1/auth/login-options");
}

export async function refresh(): Promise<LoginData> {
  return apiRequest<LoginData>("/api/v1/auth/refresh", { method: "POST" });
}

export async function logout(accessToken: string): Promise<LogoutData> {
  return authedRequest<LogoutData>("/api/v1/auth/logout", { method: "POST", accessToken }, true);
}

export async function logoutAll(accessToken: string): Promise<LogoutData> {
  return authedRequest<LogoutData>("/api/v1/auth/logout-all", { method: "POST", accessToken }, true);
}

export async function logoutSession(accessToken: string, sessionID: string): Promise<LogoutData> {
  return authedRequest<LogoutData>(
    `/api/v1/auth/sessions/${pathParam(sessionID)}/logout`,
    { method: "POST", accessToken },
    true,
  );
}

export async function getMe(accessToken: string): Promise<UserDTO> {
  const data = await authedRequest<MeData>("/api/v1/me", { accessToken }, true);
  return data.user;
}

export async function patchMe(accessToken: string, payload: PatchMePayload): Promise<UserDTO> {
  const data = await authedRequest<MeData>("/api/v1/me", { method: "PATCH", accessToken, body: payload }, true);
  return data.user;
}

export async function getCurrentActiveSessions(accessToken: string): Promise<ActiveSessionListData> {
  return authedRequest<ActiveSessionListData>("/api/v1/auth/sessions", { accessToken }, true);
}

export async function updateCurrentSessionLocation(accessToken: string, payload: UpdateCurrentSessionLocationPayload): Promise<ActiveSessionDTO> {
  return authedRequest<ActiveSessionDTO>(
    "/api/v1/auth/sessions/current/location",
    { method: "PUT", accessToken, body: payload },
    true,
  );
}
