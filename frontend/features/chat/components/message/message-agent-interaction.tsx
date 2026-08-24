"use client";

import { useTranslations } from "next-intl";
import * as React from "react";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import {
  setAgentInteractionStatus,
  updateAgentInteraction,
} from "@/features/chat/model/agent-run-store";
import { cn } from "@/lib/utils";
import {
  respondConversationInteraction,
} from "@/shared/api/conversation";
import type {
  ConversationInteractionDTO,
  ConversationInteractionResponse,
} from "@/shared/api/conversation.types";
import { resolveAccessToken } from "@/shared/auth/resolve-access-token";

type SubmitInteraction = (response: ConversationInteractionResponse) => Promise<void>;

function schemaNumber(value: string, integer: boolean): number | undefined {
  if (!value.trim()) return undefined;
  const parsed = Number(value);
  if (!Number.isFinite(parsed) || (integer && !Number.isInteger(parsed))) return undefined;
  return parsed;
}

function validImageURL(value: string): boolean {
  const trimmed = value.trim();
  if (!trimmed) return true;
  try {
    const parsed = new URL(trimmed);
    return parsed.protocol === "http:" || parsed.protocol === "https:" || parsed.protocol === "data:";
  } catch {
    return false;
  }
}

function ApprovalActions({ disabled, onSubmit }: { disabled: boolean; onSubmit: SubmitInteraction }) {
  const t = useTranslations("chat.agent");
  return (
    <div className="mt-2 flex flex-wrap items-center gap-2">
      <Button size="sm" disabled={disabled} onClick={() => void onSubmit({ kind: "approval", decision: "accept" })}>{t("interaction.accept")}</Button>
      <Button size="sm" variant="outline" disabled={disabled} onClick={() => void onSubmit({ kind: "approval", decision: "decline" })}>{t("interaction.decline")}</Button>
    </div>
  );
}

function UserInputControl({ interaction, disabled, onSubmit }: {
  interaction: Extract<ConversationInteractionDTO, { kind: "user_input" }>;
  disabled: boolean;
  onSubmit: SubmitInteraction;
}) {
  const t = useTranslations("chat.agent");
  const questions = interaction.request.questions ?? [];
  const [answers, setAnswers] = React.useState<Record<string, string>>({});
  const complete = questions.every((question) => !question.required || Boolean(answers[question.questionRef]?.trim()));
  return (
    <div className="mt-2 space-y-2">
      {questions.map((question) => {
        const label = question.question || question.prompt || question.label || question.header || t("interaction.questionFallback");
        const options = question.options ?? [];
        const answer = answers[question.questionRef] ?? "";
        const selectedOption = options.find((option) => option.label === answer);
        return (
          <div key={question.questionRef} className="min-w-0 space-y-1 text-[12px] text-muted-foreground">
            <span className="block font-medium text-foreground/85 [overflow-wrap:anywhere]">{label}</span>
            {options.length > 0 ? (
              <Select value={selectedOption?.label ?? ""} onValueChange={(value) => setAnswers((current) => ({ ...current, [question.questionRef]: value }))} disabled={disabled}>
                <SelectTrigger size="sm" className="w-full" aria-label={label}>
                  <SelectValue placeholder={t("interaction.answerPlaceholder")}>{selectedOption?.label}</SelectValue>
                </SelectTrigger>
                <SelectContent className="max-w-[calc(100vw-2rem)]">
                  {options.map((option, index) => (
                    <SelectItem key={`${option.label}:${index}`} value={option.label} className="items-start whitespace-normal">
                      <span className="min-w-0">
                        <span className="block font-medium [overflow-wrap:anywhere]">{option.label}</span>
                        {option.description ? (
                          <span className="mt-0.5 block text-[11px] leading-4 text-muted-foreground [overflow-wrap:anywhere]">
                            {option.description}
                          </span>
                        ) : null}
                      </span>
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : null}
            {options.length === 0 || question.allowFreeform ? (
              <Input
                type={question.secret ? "password" : "text"}
                value={selectedOption ? "" : answer}
                disabled={disabled}
                autoComplete={question.secret ? "off" : undefined}
                aria-label={label}
                placeholder={t("interaction.answerPlaceholder")}
                onChange={(event) => setAnswers((current) => ({ ...current, [question.questionRef]: event.target.value }))}
              />
            ) : null}
          </div>
        );
      })}
      <Button size="sm" disabled={disabled || !complete || questions.length === 0} onClick={() => void onSubmit({ kind: "user-input", answers })}>{t("interaction.submit")}</Button>
    </div>
  );
}

function PermissionControl({ interaction, disabled, onSubmit }: {
  interaction: Extract<ConversationInteractionDTO, { kind: "permission" }>;
  disabled: boolean;
  onSubmit: SubmitInteraction;
}) {
  const t = useTranslations("chat.agent");
  const scopes = interaction.request.allowedScopes?.length ? interaction.request.allowedScopes : ["turn" as const, "session" as const];
  const [scope, setScope] = React.useState<"turn" | "session">(scopes[0]);
  const [confirmOpen, setConfirmOpen] = React.useState(false);
  const permissions = Array.isArray(interaction.request.permissions)
    ? interaction.request.permissions
    : interaction.request.permissions?.names ?? [];
  return (
    <div className="mt-2 space-y-2">
      {permissions.length > 0 ? <ul className="min-w-0 list-inside list-disc text-[12px] text-muted-foreground">{permissions.map((permission) => <li key={permission} className="[overflow-wrap:anywhere]">{permission}</li>)}</ul> : null}
      {scopes.length > 1 ? (
        <Select value={scope} onValueChange={(value) => setScope(value as "turn" | "session")} disabled={disabled}>
          <SelectTrigger size="sm" className="w-full sm:w-48"><SelectValue /></SelectTrigger>
          <SelectContent>{scopes.map((item) => <SelectItem key={item} value={item}>{t(`interaction.scope.${item}`)}</SelectItem>)}</SelectContent>
        </Select>
      ) : null}
      <div className="flex flex-wrap gap-2">
        <Button size="sm" disabled={disabled} onClick={() => {
          if (scope === "session" || interaction.request.highImpact) {
            setConfirmOpen(true);
            return;
          }
          void onSubmit({ kind: "permission", decision: "accept", scope });
        }}>{t("interaction.accept")}</Button>
        <Button size="sm" variant="outline" disabled={disabled} onClick={() => void onSubmit({ kind: "permission", decision: "decline" })}>{t("interaction.decline")}</Button>
      </div>
      <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <AlertDialogContent size="compact">
          <AlertDialogHeader>
            <AlertDialogTitle>{t("interaction.confirmTitle")}</AlertDialogTitle>
            <AlertDialogDescription>{t("interaction.confirmDescription")}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("interaction.cancel")}</AlertDialogCancel>
            <AlertDialogAction variant="destructive" onClick={() => void onSubmit({ kind: "permission", decision: "accept", scope })}>{t("interaction.confirm")}</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}

function MCPElicitationControl({ interaction, disabled, onSubmit }: {
  interaction: Extract<ConversationInteractionDTO, { kind: "mcp_elicitation" }>;
  disabled: boolean;
  onSubmit: SubmitInteraction;
}) {
  const t = useTranslations("chat.agent");
  const properties = interaction.request.requestedSchema?.properties ?? {};
  const required = new Set(interaction.request.requestedSchema?.required ?? []);
  const fields = Object.entries(properties);
  const [content, setContent] = React.useState<Record<string, string | number | boolean>>({});
  const complete = fields.every(([key]) => !required.has(key) || Object.hasOwn(content, key));
  return (
    <div className="mt-2 space-y-2">
      {fields.map(([key, field]) => (
        <label key={key} className="block space-y-1 text-[12px] text-muted-foreground">
          <span className="block font-medium text-foreground/85 [overflow-wrap:anywhere]">{field.title || key}</span>
          {field.enum?.length ? (
            <Select value={content[key] === undefined ? "" : String(content[key])} onValueChange={(value) => setContent((current) => ({ ...current, [key]: field.enum?.find((item) => String(item) === value) ?? value }))} disabled={disabled}>
              <SelectTrigger size="sm" className="w-full"><SelectValue placeholder={field.description || t("interaction.answerPlaceholder")} /></SelectTrigger>
              <SelectContent className="max-w-[calc(100vw-2rem)]">{field.enum.map((value) => <SelectItem key={String(value)} value={String(value)} className="whitespace-normal [overflow-wrap:anywhere]">{String(value)}</SelectItem>)}</SelectContent>
            </Select>
          ) : field.type === "boolean" ? (
            <Select
              value={typeof content[key] === "boolean" ? String(content[key]) : ""}
              onValueChange={(value) => setContent((current) => ({ ...current, [key]: value === "true" }))}
              disabled={disabled}
            >
              <SelectTrigger size="sm" className="w-full">
                <SelectValue placeholder={field.description || t("interaction.answerPlaceholder")} />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="true">{t("interaction.boolean.true")}</SelectItem>
                <SelectItem value="false">{t("interaction.boolean.false")}</SelectItem>
              </SelectContent>
            </Select>
          ) : field.type === "number" || field.type === "integer" ? (
            <Input
              type="number"
              step={field.type === "integer" ? 1 : "any"}
              value={typeof content[key] === "number" ? content[key] : ""}
              disabled={disabled}
              placeholder={field.description || t("interaction.answerPlaceholder")}
              onChange={(event) => {
                const value = schemaNumber(event.target.value, field.type === "integer");
                setContent((current) => {
                  const next = { ...current };
                  if (value === undefined) delete next[key];
                  else next[key] = value;
                  return next;
                });
              }}
            />
          ) : (
            <Input value={typeof content[key] === "string" ? content[key] : ""} disabled={disabled} placeholder={field.description || t("interaction.answerPlaceholder")} onChange={(event) => setContent((current) => ({ ...current, [key]: event.target.value }))} />
          )}
        </label>
      ))}
      <div className="flex flex-wrap gap-2">
        <Button size="sm" disabled={disabled || !complete} onClick={() => void onSubmit({ kind: "mcp-elicitation", decision: "accept", content })}>{t("interaction.accept")}</Button>
        <Button size="sm" variant="outline" disabled={disabled} onClick={() => void onSubmit({ kind: "mcp-elicitation", decision: "decline" })}>{t("interaction.decline")}</Button>
      </div>
    </div>
  );
}

function DynamicToolControl({ interaction, disabled, onSubmit }: {
  interaction: Extract<ConversationInteractionDTO, { kind: "dynamic_tool" }>;
  disabled: boolean;
  onSubmit: SubmitInteraction;
}) {
  const t = useTranslations("chat.agent");
  const [text, setText] = React.useState("");
  const [imageURL, setImageURL] = React.useState("");
  const acceptsText = interaction.request.acceptedContentKinds?.includes("text") === true;
  const acceptsImage = interaction.request.acceptedContentKinds?.includes("image") === true;
  const imageURLValid = validImageURL(imageURL);

  const submit = React.useCallback((success: boolean) => {
    const content: Extract<ConversationInteractionResponse, { kind: "dynamic-tool" }>["content"] = [];
    const trimmedText = text.trim();
    const trimmedImageURL = imageURL.trim();
    if (acceptsText && trimmedText) content.push({ kind: "text", text: trimmedText });
    if (acceptsImage && trimmedImageURL) content.push({ kind: "image", url: trimmedImageURL });
    void onSubmit({ kind: "dynamic-tool", success, content });
  }, [acceptsImage, acceptsText, imageURL, onSubmit, text]);

  return (
    <div className="mt-2 space-y-2">
      {acceptsText ? (
        <Textarea value={text} disabled={disabled} placeholder={t("interaction.responsePlaceholder")} className="min-h-20 text-xs" onChange={(event) => setText(event.target.value)} />
      ) : null}
      {acceptsImage ? (
        <Input type="url" value={imageURL} disabled={disabled} aria-invalid={!imageURLValid} aria-label={t("interaction.imageUrlPlaceholder")} placeholder={t("interaction.imageUrlPlaceholder")} onChange={(event) => setImageURL(event.target.value)} />
      ) : null}
      <div className="flex flex-wrap gap-2">
        <Button size="sm" disabled={disabled || !imageURLValid} onClick={() => submit(true)}>{t("interaction.success")}</Button>
        <Button size="sm" variant="outline" disabled={disabled || !imageURLValid} onClick={() => submit(false)}>{t("interaction.failure")}</Button>
      </div>
    </div>
  );
}

export function MessageAgentInteractionControl({ interaction }: { interaction: ConversationInteractionDTO }) {
  const t = useTranslations("chat.agent");
  const [error, setError] = React.useState(false);
  const disabled = interaction.status === "responding" || interaction.status === "resolved";
  const submit = React.useCallback<SubmitInteraction>(async (response) => {
    setError(false);
    setAgentInteractionStatus(interaction.interactionID, "responding");
    try {
      const token = await resolveAccessToken();
      if (!token) throw new Error("missing access token");
      const result = await respondConversationInteraction(token, interaction.interactionID, response);
      updateAgentInteraction(result);
    } catch {
      setAgentInteractionStatus(interaction.interactionID, "failed");
      setError(true);
    }
  }, [interaction.interactionID]);

  const title = t(`interaction.title.${interaction.kind}`);
  const description = interaction.kind === "command_approval"
    ? interaction.request.reason
    : interaction.kind === "file_approval"
      ? interaction.request.reason
      : interaction.kind === "permission"
        ? interaction.request.description
        : interaction.kind === "mcp_elicitation"
          ? interaction.request.message || interaction.request.prompt
          : interaction.kind === "dynamic_tool"
            ? interaction.request.argumentsPreview
            : "";

  return (
    <section className="border-t border-border/35 py-3 first:border-t-0" aria-live="polite">
      <div className="flex min-w-0 items-center justify-between gap-2">
        <h4 className="min-w-0 truncate text-[12px] font-medium text-foreground/88">{title}</h4>
        <Badge variant="outline" className={cn("h-5 px-1.5 text-[10px] font-normal", interaction.status === "failed" && "border-destructive/30 text-destructive")}>{t(`interaction.status.${interaction.status}`)}</Badge>
      </div>
      {description ? <p className="mt-1 whitespace-pre-wrap text-[12px] leading-5 text-muted-foreground/72 [overflow-wrap:anywhere]">{description}</p> : null}
      {interaction.kind === "command_approval" && interaction.request.command ? <pre className="mt-2 overflow-auto whitespace-pre-wrap rounded-md bg-muted/25 px-2.5 py-2 font-mono text-[11px] leading-5 [overflow-wrap:anywhere]">{interaction.request.command}</pre> : null}
      {interaction.kind === "file_approval" ? <ul className="mt-2 space-y-0.5 font-mono text-[11px] text-muted-foreground/82">{(interaction.request.files ?? interaction.request.changes ?? []).map((file, index) => <li key={`${file.path ?? index}:${file.change ?? ""}`} className="break-all">{file.path}</li>)}</ul> : null}
      {interaction.kind === "command_approval" || interaction.kind === "file_approval" ? <ApprovalActions disabled={disabled} onSubmit={submit} /> : null}
      {interaction.kind === "user_input" ? <UserInputControl interaction={interaction} disabled={disabled} onSubmit={submit} /> : null}
      {interaction.kind === "permission" ? <PermissionControl interaction={interaction} disabled={disabled} onSubmit={submit} /> : null}
      {interaction.kind === "mcp_elicitation" ? <MCPElicitationControl interaction={interaction} disabled={disabled} onSubmit={submit} /> : null}
      {interaction.kind === "dynamic_tool" ? <DynamicToolControl interaction={interaction} disabled={disabled} onSubmit={submit} /> : null}
      {error ? <p className="mt-2 text-[11px] text-destructive">{t("interaction.responseFailed")}</p> : null}
    </section>
  );
}
