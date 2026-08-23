"use client";

import { useState, useEffect, useMemo, useRef } from "react";
import { Check, ChevronDownIcon, CircleHelp } from "lucide-react";
import { toast } from "sonner";
import { useLocale, useTranslations } from "next-intl";

import {
  Accordion,
  AccordionContent,
  AccordionItem,
  AccordionTrigger,
} from "@/components/ui/accordion";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Combobox,
  ComboboxContent,
  ComboboxEmpty,
  ComboboxInput,
  ComboboxItem,
  ComboboxList,
  ComboboxTrigger,
  ComboboxValue,
} from "@/components/ui/combobox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Sheet,
  SheetContent,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { SpinnerLabel } from "@/components/ui/spinner";
import { Textarea } from "@/components/ui/textarea";
import { cn } from "@/lib/utils";
import { resolveAccessToken } from "@/shared/auth/resolve-access-token";
import {
  createAdminLLMModel,
  getAdminReferenceData,
  invalidateAdminReferenceDataCache,
  updateAdminLLMModel,
} from "@/features/admin/api";
import {
  listModelPermissionGroups,
  listPermissionGroups,
  setModelPermissionGroups,
  type PermissionGroup,
} from "@/features/admin/api/permission-groups";
import { ModelIcon } from "@/shared/components/model-icon";
import { resolveModelIconURL, resolveModelIdentity } from "@/shared/lib/model-identity";
import type {
  AdminLLMModelDisplayGroupDTO,
  AdminLLMModelDTO,
  AdminLLMModelAccessScope,
  AdminLLMModelVendor,
  AdminLLMModelVendorDTO,
  AdminLLMStatus,
  UpdateAdminLLMModelRequest,
} from "@/features/admin/api/llm.types";

import {
  MODEL_STATUS_OPTIONS,
  MODEL_KIND_OPTIONS,
  formatDateTime,
} from "@/features/admin/types/llm";
import { resolveAdminErrorMessage } from "@/features/admin/utils/admin-error";
import {
  parseKindsJSON,
  stringifyKinds,
} from "@/shared/model/llm-schema";
import { parseProtocolsJSON } from "@/shared/lib/model-protocols";
import { JsonCodeEditor } from "@/shared/components/json-code-editor";
import {
  imageStreamEnabledFromCapabilities,
  MODEL_CAPABILITIES_PLACEHOLDER,
  ModelCapabilitiesGuideButton,
  ModelCapabilitiesQuickConfig,
  normalizeModelCapabilitiesJSON,
  setImageStreamEnabledInCapabilities,
} from "@/features/admin/components/sections/models/models-capabilities-config";
import { PermissionGroupSelector } from "@/features/admin/components/sections/groups/permission-group-selector";

// ---------------------------------------------------------------------------
// Form state
// ---------------------------------------------------------------------------

type FormState = {
  platformModelName: string;
  vendor: AdminLLMModelVendor | "";
  displayGroupID: string;
  kinds: string[];
  icon: string;
  capabilitiesJSON: string;
  systemPrompt: string;
  accessScope: AdminLLMModelAccessScope;
  status: AdminLLMStatus;
  description: string;
};

type VendorOption = {
  value: AdminLLMModelVendor;
  label: string;
  iconUrl: string | null;
};

const UNKNOWN_VENDOR = "unknown";
const FOLLOW_VENDOR_GROUP = "vendor";

const IMAGE_MEDIA_PROTOCOLS = new Set([
  "openai_image_generations",
  "openai_image_edits",
  "google_image_generation",
  "xai_image",
  "xai_image_edits",
]);

function buildInitialState(target: AdminLLMModelDTO | null): FormState {
  if (!target) {
    return {
      platformModelName: "",
      vendor: UNKNOWN_VENDOR,
      displayGroupID: FOLLOW_VENDOR_GROUP,
      kinds: [],
      icon: "",
      capabilitiesJSON: "",
      systemPrompt: "",
      accessScope: "public",
      status: "active",
      description: "",
    };
  }
  let kinds: string[] = [];
  kinds = parseKindsJSON(target.kindsJSON);
  return {
    platformModelName: target.platformModelName,
    vendor: target.vendor,
    displayGroupID: target.displayGroupID ? String(target.displayGroupID) : FOLLOW_VENDOR_GROUP,
    kinds,
    icon: target.icon ?? "",
    capabilitiesJSON: normalizeCapabilitiesText(target.capabilitiesJSON),
    systemPrompt: target.systemPrompt ?? "",
    accessScope: target.accessScope === "internal" ? "internal" : "public",
    status: target.status,
    description: target.description ?? "",
  };
}

function normalizeVendorValue(value: string): string {
  return value.trim().toLowerCase();
}

function normalizeCapabilitiesText(value: string | null | undefined): string {
  const trimmed = value?.trim() ?? "";
  return trimmed === "{}" ? "" : trimmed;
}

function VendorOptionIcon({
  iconUrl,
  label,
  unknown = false,
}: {
  iconUrl?: string | null;
  label: string;
  unknown?: boolean;
}) {
  return (
    <span className="inline-flex size-4 shrink-0 items-center justify-center self-center text-foreground">
      {iconUrl ? (
        <ModelIcon iconUrl={iconUrl} label={label} />
      ) : unknown ? (
        <CircleHelp className="size-4.5" strokeWidth={1.5} />
      ) : (
        <span className="size-2 rounded-full bg-muted-foreground/35" aria-hidden="true" />
      )}
      <span className="sr-only">{label}</span>
    </span>
  );
}

// ---------------------------------------------------------------------------
// Props
// ---------------------------------------------------------------------------

type ModelSheetProps = {
  open: boolean;
  mode: "create" | "edit";
  target: AdminLLMModelDTO | null;
  models: AdminLLMModelDTO[];
  vendors: AdminLLMModelVendorDTO[];
  displayGroups: AdminLLMModelDisplayGroupDTO[];
  onClose: () => void;
  onSuccess: () => void;
};

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export function ModelSheet({ open, mode, target, models, vendors, displayGroups, onClose, onSuccess }: ModelSheetProps) {
  const t = useTranslations("adminModels");
  const commonT = useTranslations("common");
  const locale = useLocale();
  const [form, setForm] = useState<FormState>(() => buildInitialState(target));
  const [pending, setPending] = useState(false);
  const [expandedSections, setExpandedSections] = useState<string[]>([]);
  const [showCapabilitiesJSONAdvanced, setShowCapabilitiesJSONAdvanced] = useState(false);
  const sheetContentRef = useRef<HTMLDivElement | null>(null);
  const [capabilitySourceModels, setCapabilitySourceModels] = useState<AdminLLMModelDTO[]>(models);
  const [permissionGroups, setPermissionGroups] = useState<PermissionGroup[]>([]);
  const [manualPermissionGroupIDs, setManualPermissionGroupIDs] = useState<number[]>([]);
  const [matchedPermissionGroupIDs, setMatchedPermissionGroupIDs] = useState<number[]>([]);
  const [effectivePermissionGroupIDs, setEffectivePermissionGroupIDs] = useState<number[]>([]);
  const [permissionGroupsUnassigned, setPermissionGroupsUnassigned] = useState(false);
  const [permissionGroupsLoading, setPermissionGroupsLoading] = useState(false);

  function setField<K extends keyof FormState>(key: K, value: FormState[K]) {
    setForm((prev) => ({ ...prev, [key]: value }));
  }

  function toggleKind(kind: string) {
    setForm((prev) => ({
      ...prev,
      kinds: prev.kinds.includes(kind)
        ? prev.kinds.filter((k) => k !== kind)
        : [...prev.kinds, kind],
    }));
  }


  const selectedKindLabel = form.kinds
    .map((kind) =>
      MODEL_KIND_OPTIONS.some((option) => option.value === kind)
        ? t(`kinds.${kind}`)
        : kind,
    )
    .join(", ");
  const vendorOptions = vendors.map((item) => ({
    value: item.key,
    label: item.name,
    iconUrl: resolveModelIconURL(item.icon),
  }));
  const routeProtocols = useMemo(
    () => {
      const configured = parseProtocolsJSON(target?.protocolsJSON ?? "");
      if (configured.length > 0) {
        return configured;
      }
      switch (form.vendor.trim().toLowerCase()) {
        case "openai":
        case "grok":
        case "xai":
          return ["openai_chat_completions", "openai_responses"];
        case "anthropic":
        case "claude":
          return ["anthropic_messages"];
        case "gemini":
        case "google":
        case "antigravity":
          return ["openai_chat_completions"];
        default:
          return [];
      }
    },
    [form.vendor, target?.protocolsJSON],
  );
  const imageStreamEnabled = imageStreamEnabledFromCapabilities(form.capabilitiesJSON);
  const showImageStreamControl = routeProtocols.some((protocol) => IMAGE_MEDIA_PROTOCOLS.has(protocol.trim()));
  const showPermissionGroupUnassigned =
    !permissionGroupsLoading && permissionGroupsUnassigned && effectivePermissionGroupIDs.length === 0;

  function updateImageStreamEnabled(enabled: boolean) {
    const nextValue = setImageStreamEnabledInCapabilities(form.capabilitiesJSON, enabled);
    if (nextValue === null) {
      toast.error(t("sheet.capabilitiesQuick.invalidJSON"));
      return;
    }
    setField("capabilitiesJSON", nextValue);
  }

  function handleClose() {
    onClose();
  }

  async function saveModelPermissionGroups(accessToken: string, modelID: number) {
    const data = await setModelPermissionGroups(accessToken, modelID, manualPermissionGroupIDs);
    setManualPermissionGroupIDs(data.manualGroupIDs);
    setMatchedPermissionGroupIDs(data.matchedGroupIDs);
    setEffectivePermissionGroupIDs(data.effectiveGroupIDs);
    setPermissionGroupsUnassigned(data.unassigned);
  }

  // -------------------------------------------------------------------------
  // Load when sheet opens
  // -------------------------------------------------------------------------

  useEffect(() => {
    if (!open) {
      setCapabilitySourceModels(models);
      return;
    }
    let cancelled = false;
    void (async () => {
      try {
        const token = await resolveAccessToken();
        if (!token) {
          return;
        }
        const referenceData = await getAdminReferenceData(token);
        if (!cancelled) {
          setCapabilitySourceModels(referenceData.models);
        }
      } catch {
        if (!cancelled) {
          setCapabilitySourceModels(models);
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [models, open]);

  useEffect(() => {
    if (!open) {
      setPermissionGroups([]);
      setManualPermissionGroupIDs([]);
      setMatchedPermissionGroupIDs([]);
      setEffectivePermissionGroupIDs([]);
      setPermissionGroupsUnassigned(false);
      setPermissionGroupsLoading(false);
      return;
    }

    let cancelled = false;
    setPermissionGroupsLoading(true);
    void (async () => {
      try {
        const token = await resolveAccessToken();
        if (!token) {
          return;
        }
        const [groups, modelGroups] = await Promise.all([
          listPermissionGroups(token),
          mode === "edit" && target
            ? listModelPermissionGroups(token, target.id)
            : Promise.resolve({ manualGroupIDs: [], matchedGroupIDs: [], effectiveGroupIDs: [], unassigned: false }),
        ]);
        if (cancelled) {
          return;
        }
        setPermissionGroups(groups);
        setManualPermissionGroupIDs(modelGroups.manualGroupIDs);
        setMatchedPermissionGroupIDs(modelGroups.matchedGroupIDs);
        setEffectivePermissionGroupIDs(modelGroups.effectiveGroupIDs);
        setPermissionGroupsUnassigned(modelGroups.unassigned);
      } catch (error) {
        if (!cancelled) {
          setPermissionGroups([]);
          setManualPermissionGroupIDs([]);
          setMatchedPermissionGroupIDs([]);
          setEffectivePermissionGroupIDs([]);
          setPermissionGroupsUnassigned(false);
          toast.error(t("toast.permissionGroupsLoadFailed"), { description: resolveAdminErrorMessage(error) });
        }
      } finally {
        if (!cancelled) {
          setPermissionGroupsLoading(false);
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [mode, open, t, target]);

  useEffect(() => {
    setForm(buildInitialState(open && mode === "edit" ? target : null));
    setExpandedSections(open ? ["capabilities"] : []);
    setShowCapabilitiesJSONAdvanced(false);
  }, [mode, open, target]);

  // -------------------------------------------------------------------------
  // Submit
  // -------------------------------------------------------------------------

  async function handleSubmit(e: React.FormEvent<HTMLFormElement>) {
    e.preventDefault();
    if (mode === "edit" && !target) return;

    setPending(true);
    try {
      const token = await resolveAccessToken();
      const kindsJson =
        form.kinds.length > 0 ? stringifyKinds(form.kinds) : undefined;
      if (mode === "create") {
        const data = await createAdminLLMModel(token, {
          platformModelName: form.platformModelName.trim(),
          vendor: form.vendor || undefined,
          displayGroupID: form.displayGroupID === FOLLOW_VENDOR_GROUP ? undefined : Number(form.displayGroupID),
          kindsJSON: kindsJson,
          icon: form.icon.trim() || undefined,
          capabilitiesJSON: normalizeModelCapabilitiesJSON(form.capabilitiesJSON, routeProtocols) || undefined,
          systemPrompt: form.systemPrompt.trim() || undefined,
          accessScope: form.accessScope,
          status: form.status,
          description: form.description.trim() || undefined,
        });
        if (manualPermissionGroupIDs.length > 0) {
          await saveModelPermissionGroups(token, data.model.id);
        }
        toast.success(t("toast.modelCreated"));
        setForm(buildInitialState(data.model));
        invalidateAdminReferenceDataCache();
        handleClose();
        onSuccess();
        return;
      }

      if (!target) return;
      const payload: UpdateAdminLLMModelRequest = {
        platformModelName: form.platformModelName.trim() || undefined,
        vendor: form.vendor || undefined,
        displayGroupID: form.displayGroupID === FOLLOW_VENDOR_GROUP ? 0 : Number(form.displayGroupID),
        kindsJSON: kindsJson,
        icon: form.icon.trim(),
        capabilitiesJSON: normalizeModelCapabilitiesJSON(form.capabilitiesJSON, routeProtocols),
        systemPrompt: form.systemPrompt.trim(),
        accessScope: form.accessScope,
        status: form.status,
        description: form.description.trim() || undefined,
      };
      await updateAdminLLMModel(token, target.id, payload);
      await saveModelPermissionGroups(token, target.id);
      invalidateAdminReferenceDataCache();

      handleClose();
      onSuccess();
      toast.success(t("toast.modelUpdated"));
    } catch (err) {
      toast.error(mode === "create" ? t("toast.createFailed") : t("toast.updateFailed"), { description: resolveAdminErrorMessage(err) });
    } finally {
      setPending(false);
    }
  }

  // -------------------------------------------------------------------------
  // Icon preview
  // -------------------------------------------------------------------------

  const resolvedIdentity = resolveModelIdentity({
    code: form.platformModelName,
    vendor: form.vendor,
    icon: form.icon,
  });
  const iconPreviewUrl = resolveModelIconURL(form.icon || resolvedIdentity.modelIcon);
  const selectedVendorOption =
    vendorOptions.find((item) => normalizeVendorValue(item.value) === normalizeVendorValue(form.vendor)) ??
    vendorOptions[0];

  // -------------------------------------------------------------------------
  // Render
  // -------------------------------------------------------------------------

  return (
    <Sheet open={open} onOpenChange={(nextOpen) => !nextOpen && !pending && handleClose()}>
      <SheetContent
        ref={sheetContentRef}
        className="flex flex-col gap-0 sm:max-w-[460px]"
      >
        <SheetHeader className="shrink-0 px-4 py-4">
          <SheetTitle>{mode === "create" ? t("sheet.createTitle") : t("sheet.editTitle")}</SheetTitle>
        </SheetHeader>

        <form onSubmit={handleSubmit} className="flex flex-col flex-1 min-h-0">
          <div className="min-h-0 flex-1 space-y-4 overflow-y-auto px-4 py-2">

            <div className="space-y-4">
              <div className="min-w-0 space-y-1">
                <Label className="text-xs font-normal text-muted-foreground" htmlFor="model-platform-name">{t("platformModel")}</Label>
                <Input
                  id="model-platform-name"
                  value={form.platformModelName}
                  placeholder="claude-sonnet-4.5"
                  onChange={(e) => setField("platformModelName", e.target.value)}
                  disabled={pending}
                />
              </div>

              <div className="min-w-0 space-y-1">
                <Label className="text-xs font-normal text-muted-foreground" htmlFor="model-vendor">{t("sheet.vendor")}</Label>
                <Combobox
                  id="model-vendor"
                  items={vendorOptions}
                  value={selectedVendorOption}
                  onValueChange={(item) => setField("vendor", item?.value ?? UNKNOWN_VENDOR)}
                  itemToStringLabel={(item) => item?.label ?? ""}
                  isItemEqualToValue={(item, selected) => item.value === selected.value}
                  disabled={pending}
                >
                  <ComboboxTrigger
                    render={
                      <Button
                        type="button"
                        variant="outline"
                        className="w-full justify-between border-input/40 bg-transparent px-3 py-1 font-normal hover:bg-transparent focus-visible:border-ring/60 focus-visible:ring-[1px] focus-visible:ring-ring/40 [&_[data-slot=combobox-trigger-icon]]:size-4 [&_[data-slot=combobox-trigger-icon]]:opacity-50"
                        disabled={pending}
                      >
                        <span className="flex min-w-0 flex-1 items-center justify-start gap-2">
                          <VendorOptionIcon
                            iconUrl={selectedVendorOption?.iconUrl}
                            label={selectedVendorOption?.label ?? ""}
                            unknown={selectedVendorOption?.value === UNKNOWN_VENDOR}
                          />
                          <span className="min-w-0 truncate text-left leading-5">
                            <ComboboxValue />
                          </span>
                        </span>
                      </Button>
                    }
                  />
                  <ComboboxContent
                    align="start"
                    className="min-w-[320px]"
                    portalContainer={sheetContentRef}
                  >
                    <ComboboxInput placeholder={t("sheet.vendorSearchPlaceholder")} showTrigger={false} showClear={false} disabled={pending} />
                    <ComboboxEmpty>{t("sheet.noMatchedVendors")}</ComboboxEmpty>
                    <ComboboxList>
                      {(item: VendorOption) => (
                        <ComboboxItem key={item.value} value={item} className="text-left">
                          <VendorOptionIcon
                            iconUrl={item.iconUrl}
                            label={item.label}
                            unknown={item.value === UNKNOWN_VENDOR}
                          />
                          <span className="min-w-0 flex-1 truncate leading-5">{item.label}</span>
                        </ComboboxItem>
                      )}
                    </ComboboxList>
                  </ComboboxContent>
                </Combobox>
              </div>

              <div className="min-w-0 space-y-1">
                <Label className="text-xs font-normal text-muted-foreground" htmlFor="model-display-group">
                  {t("sheet.displayGroup")}
                </Label>
                <Select
                  value={form.displayGroupID}
                  onValueChange={(value) => setField("displayGroupID", value)}
                  disabled={pending}
                >
                  <SelectTrigger id="model-display-group">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value={FOLLOW_VENDOR_GROUP}>
                      {t("sheet.followVendor", { vendor: selectedVendorOption?.label ?? form.vendor })}
                    </SelectItem>
                    {displayGroups.map((group) => (
                      <SelectItem key={group.id} value={String(group.id)}>
                        {group.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
                <p className="text-xs leading-5 text-muted-foreground">{t("sheet.displayGroupDescription")}</p>
              </div>

              <div className="min-w-0 space-y-1">
                <Label className="text-xs font-normal text-muted-foreground">{t("fields.status")}</Label>
                <Select
                  value={form.status}
                  onValueChange={(v) => setField("status", v as AdminLLMStatus)}
                  disabled={pending}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {MODEL_STATUS_OPTIONS.map((s) => (
                      <SelectItem key={s} value={s}>
                        {t(`status.${s}`)}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>

              <div className="min-w-0 space-y-1">
                <Label className="text-xs font-normal text-muted-foreground">{t("sheet.kind")}</Label>
                <Popover>
                  <PopoverTrigger asChild>
                    <Button
                      type="button"
                      variant="outline"
                      role="combobox"
                      disabled={pending}
                      className="w-full justify-between border-input/40 bg-transparent px-3 py-1 font-normal hover:bg-transparent focus-visible:border-ring/60 focus-visible:ring-[1px] focus-visible:ring-ring/40"
                    >
                      <span className={`min-w-0 flex-1 truncate text-left ${selectedKindLabel ? "" : "text-muted-foreground"}`}>
                        {selectedKindLabel || t("sheet.selectKind")}
                      </span>
                      <ChevronDownIcon className="size-3 shrink-0 text-muted-foreground opacity-50" />
                    </Button>
                  </PopoverTrigger>
                  <PopoverContent align="start" className="w-48 p-1">
                    {MODEL_KIND_OPTIONS.map(({ value }) => (
                      <button
                        key={value}
                        type="button"
                        onClick={() => toggleKind(value)}
                        className="relative flex w-full items-center rounded-sm py-1.5 pr-8 pl-2 text-xs font-normal hover:bg-accent"
                      >
                        <span className="min-w-0 flex-1 truncate text-left">{t(`kinds.${value}`)}</span>
                        <Check
                          className={`absolute right-2 size-4 shrink-0 text-muted-foreground ${
                            form.kinds.includes(value) ? "opacity-100" : "opacity-0"
                          }`}
                        />
                      </button>
                    ))}
                  </PopoverContent>
                </Popover>
              </div>

              <div className="min-w-0 space-y-1">
                <Label className="text-xs font-normal text-muted-foreground" htmlFor="model-icon">{t("sheet.icon")}</Label>
                <div className="flex items-center gap-2">
                  <Input
                    id="model-icon"
                    value={form.icon}
                    placeholder="openai"
                    onChange={(e) => setField("icon", e.target.value)}
                    disabled={pending}
                  />
                  {iconPreviewUrl ? (
                    <ModelIcon key={iconPreviewUrl} iconUrl={iconPreviewUrl} label={form.icon} size={24} />
                  ) : (
                    <div className="size-6 shrink-0" />
                  )}
                </div>
                {form.icon.trim() === "" ? (
                  <p className="text-[11px] text-muted-foreground">
                    {t("sheet.iconAutoDescription", { vendor: resolvedIdentity.vendorLabel })}
                  </p>
                ) : null}
              </div>
            </div>

            <Accordion
              type="multiple"
              value={expandedSections}
              onValueChange={setExpandedSections}
              className="border-y border-border/60"
            >
              <AccordionItem value="capabilities" className="border-border/60">
                <AccordionTrigger className="h-11 items-center py-0 text-xs font-normal text-muted-foreground hover:text-foreground hover:no-underline data-[state=open]:font-medium data-[state=open]:text-foreground [&_.accordion-trigger-icon]:translate-y-0">
                  {t("sheet.capabilities")}
                </AccordionTrigger>
                <AccordionContent className="space-y-3 pb-4 pt-0">
                  <p className="text-xs leading-5 text-muted-foreground">
                    {t("sheet.capabilitiesDescription")}
                  </p>
                  {showImageStreamControl ? (
                    <div className="pb-1">
                      <label
                        htmlFor="model-image-stream-enabled"
                        className="flex min-w-0 items-center gap-2 text-xs font-normal text-muted-foreground"
                      >
                        <Checkbox
                          id="model-image-stream-enabled"
                          checked={imageStreamEnabled}
                          disabled={pending}
                          className="size-3.5"
                          onCheckedChange={(checked) => updateImageStreamEnabled(checked === true)}
                        />
                        <span className="min-w-0 truncate">
                          {t("sheet.imageStreamEnabled")}
                        </span>
                      </label>
                    </div>
                  ) : null}
                  <div className="grid min-w-0 grid-cols-2 gap-2">
                    <ModelCapabilitiesQuickConfig
                      value={form.capabilitiesJSON}
                      disabled={pending}
                      presetModels={capabilitySourceModels}
                      currentModelID={target?.id ?? null}
                      routeProtocols={routeProtocols}
                      t={t}
                      commonT={commonT}
                      triggerVariant="secondary"
                      triggerClassName="h-8 w-full justify-start px-2 text-xs font-normal shadow-none"
                      triggerLabel={t("sheet.capabilitiesVisualButton")}
                      onApply={(nextValue) => setField("capabilitiesJSON", nextValue)}
                    />
                    <Button
                      type="button"
                      variant="secondary"
                      size="sm"
                      className="h-8 justify-between px-2 text-xs font-normal shadow-none"
                      onClick={() => setShowCapabilitiesJSONAdvanced((prev) => !prev)}
                    >
                      {t("sheet.capabilitiesAdvancedJSON")}
                      <ChevronDownIcon
                        className={cn(
                          "size-3 transition-transform",
                          showCapabilitiesJSONAdvanced && "rotate-180",
                        )}
                      />
                    </Button>
                  </div>
                  {showCapabilitiesJSONAdvanced ? (
                    <div className="space-y-1.5 pt-1">
                      <div className="flex min-w-0 items-center justify-between gap-2">
                        <p className="truncate text-[11px] text-muted-foreground">
                          {t("sheet.capabilitiesJSON")}
                        </p>
                        <ModelCapabilitiesGuideButton t={t} />
                      </div>
                      <JsonCodeEditor
                        id="model-capabilities-json"
                        value={form.capabilitiesJSON}
                        placeholder={MODEL_CAPABILITIES_PLACEHOLDER}
                        height={220}
                        onChange={(nextValue) => setField("capabilitiesJSON", nextValue)}
                        disabled={pending}
                      />
                    </div>
                  ) : null}
                </AccordionContent>
              </AccordionItem>


              <AccordionItem value="other" className="border-border/60">
                <AccordionTrigger className="h-11 items-center py-0 text-xs font-normal text-muted-foreground hover:text-foreground hover:no-underline data-[state=open]:font-medium data-[state=open]:text-foreground [&_.accordion-trigger-icon]:translate-y-0">
                  {t("sheet.otherInfo")}
                </AccordionTrigger>
                <AccordionContent className="space-y-4 pb-4 pt-0">
                  <div className="space-y-1">
                    <Label className="text-xs font-normal text-muted-foreground">{t("sheet.accessScope")}</Label>
                    <Select
                      value={form.accessScope}
                      onValueChange={(v) => setField("accessScope", v as AdminLLMModelAccessScope)}
                      disabled={pending}
                    >
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value="public">{t("accessScope.public")}</SelectItem>
                        <SelectItem value="internal">{t("accessScope.internal")}</SelectItem>
                      </SelectContent>
                    </Select>
                  </div>

                  <div className="space-y-1.5">
                    <Label className="text-xs font-normal text-muted-foreground">
                      {t("sheet.permissionGroups")}
                    </Label>
                    <PermissionGroupSelector
                      groups={permissionGroups}
                      selectedIDs={manualPermissionGroupIDs}
                      matchedIDs={matchedPermissionGroupIDs}
                      disabled={pending}
                      loading={permissionGroupsLoading}
                      placeholder={t("sheet.permissionGroupsPlaceholder")}
                      emptyLabel={t("sheet.permissionGroupsEmpty")}
                      autoBadgeLabel={t("sheet.permissionGroupsAutoBadge")}
                      onSelectedIDsChange={setManualPermissionGroupIDs}
                    />
                    <p className={cn("text-[11px] leading-4", showPermissionGroupUnassigned ? "text-destructive" : "text-muted-foreground")}>
                      {showPermissionGroupUnassigned
                        ? t("sheet.permissionGroupsUnassigned")
                        : t("sheet.permissionGroupsDescription")}
                    </p>
                  </div>

                  <div className="space-y-1">
                    <Label className="text-xs font-normal text-muted-foreground" htmlFor="model-desc">{t("sheet.description")}</Label>
                    <Textarea
                      id="model-desc"
                      value={form.description}
                      placeholder={t("sheet.descriptionPlaceholder")}
                      className="h-20 resize-none overflow-y-auto [field-sizing:fixed]"
                      onChange={(e) => setField("description", e.target.value)}
                      disabled={pending}
                    />
                  </div>

                  <div className="space-y-1">
                    <Label className="text-xs font-normal text-muted-foreground" htmlFor="model-system-prompt">{t("sheet.systemPrompt")}</Label>
                    <Textarea
                      id="model-system-prompt"
                      value={form.systemPrompt}
                      placeholder={t("sheet.systemPromptPlaceholder")}
                      className="h-28 resize-none overflow-y-auto [field-sizing:fixed]"
                      onChange={(e) => setField("systemPrompt", e.target.value)}
                      disabled={pending}
                      maxLength={20000}
                    />
                    <p className="mt-1 text-[11px] text-muted-foreground">
                      {t("sheet.systemPromptDescription")}
                    </p>
                  </div>
                </AccordionContent>
              </AccordionItem>

              {target && (
                <AccordionItem value="meta" className="border-border/60">
                  <AccordionTrigger className="h-11 items-center py-0 text-xs font-normal text-muted-foreground hover:text-foreground hover:no-underline data-[state=open]:font-medium data-[state=open]:text-foreground [&_.accordion-trigger-icon]:translate-y-0">
                    {t("sheet.metadata")}
                  </AccordionTrigger>
                  <AccordionContent className="space-y-2 pb-4 pt-0 text-xs">
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">ID</span>
                      <span className="font-mono">{target.id}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">{t("sheet.createdAt")}</span>
                      <span>{formatDateTime(target.createdAt, locale)}</span>
                    </div>
                    <div className="flex justify-between">
                      <span className="text-muted-foreground">{t("sheet.updatedAt")}</span>
                      <span>{formatDateTime(target.updatedAt, locale)}</span>
                    </div>
                  </AccordionContent>
                </AccordionItem>
              )}
            </Accordion>
          </div>

          <SheetFooter className="flex flex-row justify-end px-4 py-3 gap-2">
            <Button
              type="button"
              variant="ghost"
              onClick={handleClose}
              disabled={pending}
            >
              {commonT("actions.cancel")}
            </Button>
            <Button type="submit" disabled={pending}>
              {pending ? <SpinnerLabel>{t("sheet.saving")}</SpinnerLabel> : commonT("actions.save")}
            </Button>
          </SheetFooter>
        </form>
      </SheetContent>
    </Sheet>
  );
}
