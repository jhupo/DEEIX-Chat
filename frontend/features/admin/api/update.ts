import { authedRequest } from "@/shared/api/authed-client";

export type AdminUpdateCandidate = {
  version: string;
  tag: string;
  releaseURL: string;
  manifestDigest: string;
  bundleURL: string;
  bundleDigest: string;
  bundleSize: number;
  commit: string;
  publishedAt: string;
};

export type AdminUpdateJob = {
  id: string;
  version: string;
  status: "queued" | "pulling" | "applying" | "verifying" | "succeeded" | "failed" | "outcome_unknown";
  error?: string;
  createdAt: string;
  updatedAt: string;
};

export type AdminUpdateStatus = {
  installedVersion: string;
  candidate?: AdminUpdateCandidate;
  updateAvailable: boolean;
  job?: AdminUpdateJob;
};

export function getAdminUpdateStatus(accessToken: string) {
  return authedRequest<AdminUpdateStatus>("/api/v1/admin/update/status", { accessToken }, true);
}

export function checkAdminUpdate(accessToken: string) {
  return authedRequest<AdminUpdateStatus>("/api/v1/admin/update/check", { method: "POST", accessToken }, true);
}

export function installAdminUpdate(accessToken: string, idempotencyKey: string, input: { version: string; manifestDigest: string; confirmation: string }) {
  return authedRequest<AdminUpdateJob>("/api/v1/admin/update/install", {
    method: "POST",
    accessToken,
    headers: { "Idempotency-Key": idempotencyKey },
    body: input,
  }, true);
}

export function getAdminUpdateJob(accessToken: string, jobID: string) {
  return authedRequest<AdminUpdateJob>(`/api/v1/admin/update/jobs/${encodeURIComponent(jobID)}`, { accessToken }, true);
}

export function restartAdminUpdate(accessToken: string) {
  return authedRequest<{ restarting: boolean }>("/api/v1/admin/update/restart", { method: "POST", accessToken }, true);
}
