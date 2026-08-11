import type {
  ActiveSessionListResponse,
  ActiveSessionResponse,
  AuthUserResponse,
  EmailRegistrationStartResponse,
  LoginOptionsResponse,
  LoginResponse,
  LogoutResponse,
  PatchMeRequest,
  UpdateCurrentSessionLocationRequest,
} from "@deeix/api-contract";

export type UserDTO = AuthUserResponse;

export type LoginData = LoginResponse;

export type EmailRegistrationStartData = EmailRegistrationStartResponse & {
  debugCode?: string;
};

export type LoginPageSettings = {
  defaultNextPath: string;
};

export type LoginOptionsData = Pick<
  LoginOptionsResponse,
  "emailRegistrationEnabled" | "emailVerificationEnabled" | "turnstileEnabled" | "turnstileSiteKey"
>;

export type MeData = {
  user: UserDTO;
};

export type PatchMePayload = PatchMeRequest;

export type ChangePasswordPayload = {
  currentPassword: string;
  newPassword: string;
};

export type ChangePasswordData = {
  changed: boolean;
};

export type LogoutData = LogoutResponse;

export type ActiveSessionDTO = ActiveSessionResponse;

export type ActiveSessionListData = Omit<ActiveSessionListResponse, "results"> & {
  results: ActiveSessionDTO[];
};

export type UpdateCurrentSessionLocationPayload = UpdateCurrentSessionLocationRequest;
