"use client";

import { Brain, Check, ChevronDown, ChevronLeft, ChevronRight } from "lucide-react";
import { useTranslations } from "next-intl";
import * as React from "react";
import { InputGroupButton } from "@/components/ui/input-group";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { Skeleton } from "@/components/ui/skeleton";
import type { ChatModelOption } from "@/features/chat/types/chat-runtime";
import { cn } from "@/lib/utils";
import { ModelIcon } from "@/shared/components/model-icon";
import { useIsMobile } from "@/shared/hooks/use-mobile";
import { resolveModelIconURL, resolveModelIdentity } from "@/shared/lib/model-identity";
import { resolveModelPresentationGroup } from "@/shared/lib/model-presentation";
import {
  resolveDesktopMenuListMaxHeight,
  resolveDesktopModelMenuListMaxHeight,
} from "./chat-model-picker-layout";

type ChatModelPickerProps = {
  modelOptions: ChatModelOption[];
  selectedPlatformModelName: string;
  reasoning?: ChatModelPickerReasoning;
  loading: boolean;
  disabled: boolean;
  onModelCatalogRefresh?: () => void | Promise<void>;
  onModelChange: (platformModelName: string) => void;
};

export type ChatModelPickerReasoning = {
  value: string;
  options: string[];
  disabled?: boolean;
  onChange: (value: string) => void;
};

const MODEL_MENU_COLLISION_PADDING = 24;
const DESKTOP_MODEL_MENU_WIDTH = 224;
const DESKTOP_MODEL_SUBMENU_GAP = 8;
const DESKTOP_MODEL_MENU_SIDE_OFFSET = 8;
const DESKTOP_MODEL_MENU_MIN_SCROLL_HEIGHT = 96;
/** Group panel non-list chrome: p-1.5 × 2 + header h-7. */
const DESKTOP_GROUP_MENU_VERTICAL_CHROME = 40;
const DESKTOP_REASONING_MENU_VERTICAL_CHROME = 32;
/** Model submenu non-list chrome: p-1.5 × 2. */
const DESKTOP_SUBMENU_VERTICAL_CHROME = 12;
const REASONING_MENU_KEY = "__reasoning__";

function resolveModelGroups(modelOptions: ChatModelOption[]) {
  const groupMap = new Map<string, { label: string; icon: string; items: ChatModelOption[] }>();
  for (const item of modelOptions) {
    const presentation = resolveModelPresentationGroup(item);
    const group = groupMap.get(presentation.key);
    if (group) {
      group.items.push(item);
      continue;
    }
    groupMap.set(presentation.key, {
      label: presentation.label,
      icon: presentation.icon,
      items: [item],
    });
  }

  return Array.from(groupMap.entries()).map(([key, group]) => ({ key, ...group }));
}

function ChatModelIdentity({
  model,
  density = "default",
}: {
  model: ChatModelOption;
  density?: "default" | "compact";
}) {
  const platformModelName = model.platformModelName.trim();
  const displayName = model.displayName?.trim() || platformModelName;
  const identity = React.useMemo(
    () =>
      resolveModelIdentity({
        code: model.platformModelName,
        vendor: model.vendor,
        icon: model.icon,
      }),
    [model.icon, model.platformModelName, model.vendor],
  );
  const iconURL = React.useMemo(() => resolveModelIconURL(identity.modelIcon), [identity.modelIcon]);
  const compact = density === "compact";

  return (
    <div className={cn("flex min-w-0 items-center", compact ? "gap-2" : "gap-2.5")}>
      <ModelIcon iconUrl={iconURL} label={displayName} />
      <div className="min-w-0 flex-1 overflow-hidden">
        <div className={cn("flex items-center", compact ? "gap-1" : "gap-1.5")}>
          <p
            className={cn(
              "truncate font-medium text-foreground",
              compact ? "text-[12.5px] leading-4" : "text-[13px] leading-4.5",
            )}
          >
            {displayName}
          </p>
        </div>
      </div>
    </div>
  );
}

function ChatModelTriggerSkeleton() {
  return (
    <div className="flex min-w-0 items-center gap-2.5">
      <Skeleton className="size-4 shrink-0 rounded-full bg-muted/55" />
      <Skeleton className="h-3.5 w-20 rounded-full bg-muted/50" />
    </div>
  );
}

function ModelMenuScrollContainer({
  children,
  maxHeight,
  onScroll,
}: {
  children: React.ReactNode;
  maxHeight?: number;
  onScroll?: () => void;
}) {
  const viewportRef = React.useRef<HTMLDivElement | null>(null);
  const [hasMoreAbove, setHasMoreAbove] = React.useState(false);
  const [hasMoreBelow, setHasMoreBelow] = React.useState(false);
  const resolvedMaxHeight = Number.isFinite(maxHeight) ? Math.max(0, maxHeight ?? 0) : undefined;

  const updateScrollHints = React.useCallback(() => {
    const viewport = viewportRef.current;
    if (!viewport) {
      setHasMoreAbove(false);
      setHasMoreBelow(false);
      return;
    }
    const remaining = viewport.scrollHeight - viewport.clientHeight - viewport.scrollTop;
    setHasMoreAbove(viewport.scrollTop > 1);
    setHasMoreBelow(remaining > 1);
  }, []);

  React.useLayoutEffect(() => {
    updateScrollHints();
    const viewport = viewportRef.current;
    if (!viewport || typeof ResizeObserver === "undefined") {
      return;
    }

    const observer = new ResizeObserver(updateScrollHints);
    observer.observe(viewport);
    if (viewport.firstElementChild) {
      observer.observe(viewport.firstElementChild);
    }
    return () => observer.disconnect();
  }, [children, resolvedMaxHeight, updateScrollHints]);

  const handleScroll = React.useCallback(() => {
    updateScrollHints();
    onScroll?.();
  }, [onScroll, updateScrollHints]);

  return (
    <div className="relative">
      <div
        ref={viewportRef}
        style={resolvedMaxHeight === undefined ? undefined : { maxHeight: resolvedMaxHeight }}
        className={cn(
          "overflow-y-auto overscroll-contain pr-0 [-ms-overflow-style:none] [scrollbar-width:none] [&::-webkit-scrollbar]:hidden",
          resolvedMaxHeight === undefined
            ? "max-h-[min(20rem,var(--model-menu-scroll-max-height,var(--radix-popover-content-available-height)))]"
            : null,
        )}
        onScroll={handleScroll}
      >
        {children}
      </div>
      {hasMoreAbove ? (
        <div className="pointer-events-none absolute inset-x-0 top-0 flex h-4 items-start justify-center rounded-t-lg bg-gradient-to-b from-popover via-popover/80 to-transparent pt-px">
          <ChevronDown className="size-3 rotate-180 text-muted-foreground/75" strokeWidth={1.8} />
        </div>
      ) : null}
      {hasMoreBelow ? (
        <div className="pointer-events-none absolute inset-x-0 bottom-0 flex h-4 items-end justify-center rounded-b-lg bg-gradient-to-t from-popover via-popover/80 to-transparent pb-px">
          <ChevronDown className="size-3 text-muted-foreground/75" strokeWidth={1.8} />
        </div>
      ) : null}
    </div>
  );
}

function ChatModelMenuItem({
  model,
  selected,
  onSelect,
  buttonRef,
}: {
  model: ChatModelOption;
  selected: boolean;
  onSelect: () => void;
  buttonRef?: React.Ref<HTMLButtonElement>;
}) {
  const platformModelName = model.platformModelName.trim();
  const displayName = model.displayName?.trim() || platformModelName;
  const identity = React.useMemo(
    () =>
      resolveModelIdentity({
        code: model.platformModelName,
        vendor: model.vendor,
        icon: model.icon,
      }),
    [model.icon, model.platformModelName, model.vendor],
  );
  const iconURL = React.useMemo(() => resolveModelIconURL(identity.modelIcon), [identity.modelIcon]);

  return (
    <div
      data-selected={selected}
      className="group flex h-7 items-center rounded-md text-[11px] font-medium text-muted-foreground transition-colors hover:bg-accent hover:text-accent-foreground focus-within:bg-accent focus-within:text-accent-foreground data-[selected=true]:bg-accent data-[selected=true]:text-accent-foreground"
    >
      <button
        ref={buttonRef}
        type="button"
        className="flex h-7 min-w-0 flex-1 items-center gap-2 rounded-md bg-transparent py-0 pl-2 pr-1 text-left text-[11px] font-medium leading-none text-inherit outline-none"
        title={model.description?.trim() || displayName}
        onClick={onSelect}
      >
        <ModelIcon iconUrl={iconURL} label={displayName} />
        <span className="min-w-0 flex-1 truncate leading-4">
          {displayName}
        </span>
        <span className="flex size-3 shrink-0 items-center justify-center">
          {selected ? <Check className="size-3 text-current" strokeWidth={1.7} /> : null}
        </span>
      </button>
    </div>
  );
}

function ReasoningMenuItems({
  reasoning,
  formatLabel,
  onSelect,
}: {
  reasoning: ChatModelPickerReasoning;
  formatLabel: (value: string) => string;
  onSelect: (value: string) => void;
}) {
  return (
    <div className="flex flex-col gap-0.5">
      {reasoning.options.map((option) => (
        <button
          key={option}
          type="button"
          className="flex h-7 w-full items-center gap-2 rounded-md px-2 text-left text-[11px] font-medium text-muted-foreground outline-none transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:bg-accent focus-visible:text-accent-foreground"
          aria-pressed={option === reasoning.value}
          onClick={() => onSelect(option)}
        >
          <span className="min-w-0 flex-1 truncate">{formatLabel(option)}</span>
          <span className="flex size-3 shrink-0 items-center justify-center">
            {option === reasoning.value ? <Check className="size-3 text-current" strokeWidth={1.7} /> : null}
          </span>
        </button>
      ))}
    </div>
  );
}

export function ChatModelPicker({
  modelOptions,
  selectedPlatformModelName,
  reasoning,
  loading,
  disabled,
  onModelCatalogRefresh,
  onModelChange,
}: ChatModelPickerProps) {
  const t = useTranslations("chat.modelPicker");
  const isMobile = useIsMobile();
  const [open, setOpen] = React.useState(false);
  const [activeGroupKey, setActiveGroupKey] = React.useState("");
  const [mobileGroupKey, setMobileGroupKey] = React.useState<string | null>(null);
  const [desktopSubmenuSide, setDesktopSubmenuSide] = React.useState<"right" | "left">("right");
  const [desktopSubmenuTop, setDesktopSubmenuTop] = React.useState(0);
  const [desktopSubmenuWidth, setDesktopSubmenuWidth] = React.useState(DESKTOP_MODEL_MENU_WIDTH);
  const [desktopGroupListMaxHeight, setDesktopGroupListMaxHeight] = React.useState(320);
  const [desktopSubmenuListMaxHeight, setDesktopSubmenuListMaxHeight] = React.useState(320);
  const desktopMenuRootRef = React.useRef<HTMLDivElement | null>(null);
  const desktopGroupMenuRef = React.useRef<HTMLDivElement | null>(null);
  const desktopSubmenuRef = React.useRef<HTMLDivElement | null>(null);
  const selectedModelButtonRef = React.useRef<HTMLButtonElement | null>(null);
  const reasoningButtonRef = React.useRef<HTMLButtonElement | null>(null);
  const desktopGroupItemRefs = React.useRef(new Map<string, HTMLButtonElement>());
  const selectedModel = React.useMemo(
    () => modelOptions.find((item) => item.platformModelName === selectedPlatformModelName) ?? null,
    [modelOptions, selectedPlatformModelName],
  );
  const selectedGroupKey = React.useMemo(() => {
    if (!selectedModel) {
      return "";
    }
    return resolveModelPresentationGroup(selectedModel).key;
  }, [selectedModel]);
  const selectedGroupLabel = React.useMemo(() => {
    if (!selectedModel) {
      return "none";
    }
    return resolveModelPresentationGroup(selectedModel).label;
  }, [selectedModel]);
  const modelGroups = React.useMemo(() => resolveModelGroups(modelOptions), [modelOptions]);
  const reasoningAvailable = Boolean(reasoning && reasoning.options.length > 0);
  const reasoningActive = reasoningAvailable && activeGroupKey === REASONING_MENU_KEY;
  const formatReasoningLabel = React.useCallback(
    (value: string) => t.has(`efforts.${value}`) ? t(`efforts.${value}`) : value,
    [t],
  );
  const reasoningValueLabel = reasoning ? formatReasoningLabel(reasoning.value) : "";
  const activeDesktopGroupKey = activeGroupKey && activeGroupKey !== REASONING_MENU_KEY
    ? activeGroupKey
    : selectedGroupKey || modelGroups[0]?.key || "";
  const activeDesktopGroup = React.useMemo(
    () => modelGroups.find((group) => group.key === activeDesktopGroupKey) ?? modelGroups[0] ?? null,
    [activeDesktopGroupKey, modelGroups],
  );
  const hasDesktopSubmenu = reasoningActive || Boolean(activeDesktopGroup?.items.length);
  const mobileGroup = React.useMemo(
    () => modelGroups.find((group) => group.key === mobileGroupKey) ?? null,
    [mobileGroupKey, modelGroups],
  );
  const mobileReasoningOpen = reasoningAvailable && mobileGroupKey === REASONING_MENU_KEY;
  React.useEffect(() => {
    if (!open || !isMobile) {
      setMobileGroupKey(null);
    }
  }, [isMobile, open]);
  React.useEffect(() => {
    if (disabled || loading) {
      setOpen(false);
    }
  }, [disabled, loading]);

  const updateDesktopSubmenuMetrics = React.useCallback(() => {
    if (!open || isMobile) {
      setDesktopSubmenuSide("right");
      setDesktopSubmenuTop(0);
      setDesktopSubmenuWidth(DESKTOP_MODEL_MENU_WIDTH);
      setDesktopGroupListMaxHeight(320);
      setDesktopSubmenuListMaxHeight(320);
      return;
    }

    const menuRoot = desktopMenuRootRef.current;
    const groupMenu = desktopGroupMenuRef.current;
    if (!menuRoot || !groupMenu) {
      return;
    }

    const menuRootRect = menuRoot.getBoundingClientRect();
    const groupMenuRect = groupMenu.getBoundingClientRect();
    const submenu = desktopSubmenuRef.current;
    const submenuRect = submenu?.getBoundingClientRect();
    const activeGroupButton = reasoningActive
      ? reasoningButtonRef.current
      : activeDesktopGroup
        ? desktopGroupItemRefs.current.get(activeDesktopGroup.key)
        : null;
    const activeGroupRect = activeGroupButton?.getBoundingClientRect();
    const triggerRect = document.getElementById("chat-model-menu-trigger")?.getBoundingClientRect();
    const viewportLeft = MODEL_MENU_COLLISION_PADDING;
    const viewportRight = window.innerWidth - MODEL_MENU_COLLISION_PADDING;
    const viewportTop = MODEL_MENU_COLLISION_PADDING;
    const viewportBottom = window.innerHeight - MODEL_MENU_COLLISION_PADDING;
    const viewportHeight = Math.max(0, viewportBottom - viewportTop);
    const rightAvailableWidth = Math.max(0, viewportRight - groupMenuRect.right - DESKTOP_MODEL_SUBMENU_GAP);
    const leftAvailableWidth = Math.max(0, groupMenuRect.left - viewportLeft - DESKTOP_MODEL_SUBMENU_GAP);
    const nextSubmenuSide =
      rightAvailableWidth >= DESKTOP_MODEL_MENU_WIDTH || rightAvailableWidth >= leftAvailableWidth
        ? "right"
        : "left";
    const nextSubmenuWidth = Math.max(
      DESKTOP_MODEL_MENU_MIN_SCROLL_HEIGHT,
      Math.min(
        DESKTOP_MODEL_MENU_WIDTH,
        nextSubmenuSide === "right" ? rightAvailableWidth : leftAvailableWidth,
      ),
    );
    const nextGroupListMaxHeight = resolveDesktopModelMenuListMaxHeight({
      viewportTop,
      viewportBottom,
      triggerTop: triggerRect?.top,
      triggerBottom: triggerRect?.bottom,
      sideOffset: DESKTOP_MODEL_MENU_SIDE_OFFSET,
      verticalChrome: DESKTOP_GROUP_MENU_VERTICAL_CHROME +
        (reasoningAvailable ? DESKTOP_REASONING_MENU_VERTICAL_CHROME : 0),
    });

    let nextSubmenuTop = 0;
    let nextSubmenuListMaxHeight = nextGroupListMaxHeight;
    if (hasDesktopSubmenu && activeGroupRect) {
      const submenuHeight = submenuRect?.height ?? nextGroupListMaxHeight + DESKTOP_SUBMENU_VERTICAL_CHROME;
      const submenuOuterHeight = Math.min(submenuHeight, viewportHeight);
      const maxViewportTop = Math.max(viewportTop, viewportBottom - submenuOuterHeight);
      // Anchor in viewport coordinates, then convert to an offset within
      // menuRoot (which may sit above viewportTop for a frame before re-shift).
      const preferredSubmenuViewportTop = Math.min(
        Math.max(activeGroupRect.top, viewportTop),
        maxViewportTop,
      );
      nextSubmenuTop = preferredSubmenuViewportTop - menuRootRect.top;
      const actualSubmenuViewportTop = menuRootRect.top + nextSubmenuTop;
      nextSubmenuListMaxHeight = resolveDesktopMenuListMaxHeight(
        Math.min(viewportHeight, viewportBottom - actualSubmenuViewportTop),
        DESKTOP_SUBMENU_VERTICAL_CHROME,
      );
    }

    setDesktopSubmenuSide(nextSubmenuSide);
    setDesktopSubmenuTop(nextSubmenuTop);
    setDesktopSubmenuWidth(nextSubmenuWidth);
    setDesktopGroupListMaxHeight(nextGroupListMaxHeight);
    setDesktopSubmenuListMaxHeight(nextSubmenuListMaxHeight);
  }, [activeDesktopGroup, hasDesktopSubmenu, isMobile, open, reasoningActive, reasoningAvailable]);

  React.useLayoutEffect(() => {
    updateDesktopSubmenuMetrics();

    if (!open || isMobile) {
      return;
    }

    window.addEventListener("resize", updateDesktopSubmenuMetrics);
    window.addEventListener("scroll", updateDesktopSubmenuMetrics, true);
    if (typeof ResizeObserver === "undefined") {
      return () => {
        window.removeEventListener("resize", updateDesktopSubmenuMetrics);
        window.removeEventListener("scroll", updateDesktopSubmenuMetrics, true);
      };
    }

    const observer = new ResizeObserver(updateDesktopSubmenuMetrics);
    if (desktopMenuRootRef.current) {
      observer.observe(desktopMenuRootRef.current);
    }
    if (desktopGroupMenuRef.current) {
      observer.observe(desktopGroupMenuRef.current);
    }
    if (desktopSubmenuRef.current) {
      observer.observe(desktopSubmenuRef.current);
    }
    const activeGroupButton = reasoningActive
      ? reasoningButtonRef.current
      : activeDesktopGroup
        ? desktopGroupItemRefs.current.get(activeDesktopGroup.key)
        : null;
    if (activeGroupButton) {
      observer.observe(activeGroupButton);
    }

    return () => {
      window.removeEventListener("resize", updateDesktopSubmenuMetrics);
      window.removeEventListener("scroll", updateDesktopSubmenuMetrics, true);
      observer.disconnect();
    };
  }, [activeDesktopGroup, hasDesktopSubmenu, isMobile, open, reasoningActive, updateDesktopSubmenuMetrics]);

  const handleOpenChange = React.useCallback(
    (nextOpen: boolean) => {
      if (nextOpen) {
        setActiveGroupKey(selectedGroupKey || modelGroups[0]?.key || "");
        if (onModelCatalogRefresh) {
          void Promise.resolve(onModelCatalogRefresh()).catch((): undefined => undefined);
        }
      }
      setOpen(nextOpen);
    },
    [modelGroups, onModelCatalogRefresh, selectedGroupKey],
  );

  const closeMenu = React.useCallback(() => {
    handleOpenChange(false);
  }, [handleOpenChange]);

  const selectDesktopGroup = React.useCallback((groupKey: string) => {
    if (groupKey === activeGroupKey) {
      return;
    }
    setActiveGroupKey(groupKey);
  }, [activeGroupKey]);

  return (
    <>
      <div className="min-w-0 max-w-[min(320px,100%)] shrink">
        <Popover open={open} onOpenChange={handleOpenChange}>
          <PopoverTrigger asChild>
            <InputGroupButton
              id="chat-model-menu-trigger"
              type="button"
              variant="ghost"
              size="sm"
              className="w-full min-w-0 max-w-[min(320px,100%)] rounded-lg px-1.5 hover:bg-accent focus-visible:bg-accent data-[state=open]:bg-accent sm:px-2"
              disabled={disabled || loading}
              aria-label={t("selectModel")}
            >
              {loading ? (
                <ChatModelTriggerSkeleton />
              ) : (
                <>
                  <span className="min-w-0 flex-1">
                    {selectedModel ? (
                      <ChatModelIdentity model={selectedModel} density="compact" />
                    ) : selectedPlatformModelName.trim() ? (
                      <span className="block truncate text-[12px] font-medium text-foreground">
                        {selectedPlatformModelName}
                      </span>
                    ) : (
                      <span className="block truncate text-[12px] font-medium text-muted-foreground">
                        {t("selectModel")}
                      </span>
                    )}
                  </span>
                  {reasoningAvailable ? (
                    <span className="shrink-0 text-[10px] font-medium text-muted-foreground">
                      {reasoningValueLabel}
                    </span>
                  ) : null}
                  <ChevronDown className="size-3 shrink-0 text-muted-foreground/70" strokeWidth={1.8} />
                </>
              )}
            </InputGroupButton>
          </PopoverTrigger>
          <PopoverContent
            align="end"
            side="bottom"
            sideOffset={DESKTOP_MODEL_MENU_SIDE_OFFSET}
            collisionPadding={24}
            onOpenAutoFocus={(event) => {
              if (!isMobile && selectedModelButtonRef.current) {
                event.preventDefault();
                selectedModelButtonRef.current.focus();
              }
            }}
            className={cn(
              "relative overflow-visible rounded-xl",
              isMobile
                ? "w-[min(20rem,calc(100vw-3rem))] p-1.5"
                : "w-[min(14rem,calc(100vw-3rem))] border-0 bg-transparent p-0 shadow-none",
            )}
          >
            {isMobile ? (
              <>
                <div className="flex h-7 items-center justify-between gap-2 px-2">
                  {mobileGroup || mobileReasoningOpen ? (
                    <button
                      type="button"
                      className="-ml-1.5 flex h-7 min-w-0 items-center gap-0.5 rounded-md px-0.5 text-[11px] font-medium text-muted-foreground outline-none transition-colors hover:bg-accent hover:text-foreground focus-visible:bg-accent focus-visible:text-foreground"
                      onClick={() => setMobileGroupKey(null)}
                    >
                      <ChevronLeft className="size-3.5" strokeWidth={1.8} />
                      <span>{mobileReasoningOpen ? t("reasoning") : t("group")}</span>
                    </button>
                  ) : (
                    <span className="text-[11px] font-medium text-foreground">{t("group")}</span>
                  )}
                  <span className="min-w-0 truncate text-right text-[10px] font-medium text-muted-foreground">
                    {mobileReasoningOpen ? reasoningValueLabel : mobileGroup ? mobileGroup.label : selectedGroupLabel}
                  </span>
                </div>
                <ModelMenuScrollContainer>
                  {mobileReasoningOpen && reasoning ? (
                    <ReasoningMenuItems
                      reasoning={reasoning}
                      formatLabel={formatReasoningLabel}
                      onSelect={(value) => {
                        reasoning.onChange(value);
                        closeMenu();
                      }}
                    />
                  ) : mobileGroup ? (
                      <div className="flex flex-col gap-0.5">
                        {mobileGroup.items.map((item) => (
                          <ChatModelMenuItem
                            key={item.platformModelName}
                            model={item}
                            selected={item.platformModelName === selectedPlatformModelName}
                            onSelect={() => {
                              onModelChange(item.platformModelName);
                              closeMenu();
                            }}
                          />
                        ))}
                      </div>
                  ) : (
                    <>
                      <div className="flex flex-col gap-0.5">
                        {modelGroups.length === 0 ? (
                          <div className="px-2 py-3 text-[11px] leading-4 text-muted-foreground">
                            {t("empty")}
                          </div>
                        ) : modelGroups.map((group) => {
                          const selectedGroup = group.key === selectedGroupKey;
                          const groupIconURL = resolveModelIconURL(group.icon);
                          return (
                            <button
                              type="button"
                              key={group.key}
                              className={cn(
                                "flex h-7 w-full items-center justify-between gap-2 rounded-md px-2 py-0 text-left text-[11px] font-medium outline-none transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:bg-accent focus-visible:text-accent-foreground",
                                selectedGroup ? "bg-accent text-accent-foreground" : "text-muted-foreground",
                              )}
                              onClick={() => {
                                setMobileGroupKey(group.key);
                              }}
                            >
                              <ModelIcon iconUrl={groupIconURL} label={group.label} />
                              <span className="min-w-0 flex-1 truncate font-medium">{group.label}</span>
                              <span className="shrink-0 text-[10px] tabular-nums text-muted-foreground/80">
                                {group.items.length}
                              </span>
                            </button>
                          );
                        })}
                      </div>
                      {reasoningAvailable ? (
                        <div className="mt-1 border-t border-border/60 pt-1">
                          <button
                            type="button"
                            className="flex h-7 w-full items-center gap-2 rounded-md px-2 text-left text-[11px] font-medium text-muted-foreground outline-none transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:bg-accent focus-visible:text-accent-foreground disabled:pointer-events-none disabled:opacity-50"
                            disabled={reasoning?.disabled}
                            onClick={() => setMobileGroupKey(REASONING_MENU_KEY)}
                          >
                            <Brain className="size-3.5 shrink-0" strokeWidth={1.7} />
                            <span className="min-w-0 flex-1 truncate">{t("reasoning")}</span>
                            <span className="shrink-0 text-[10px] text-muted-foreground/80">{reasoningValueLabel}</span>
                            <ChevronRight className="size-3.5 shrink-0 text-muted-foreground/65" strokeWidth={1.8} />
                          </button>
                        </div>
                      ) : null}
                    </>
                  )}
                </ModelMenuScrollContainer>
              </>
            ) : (
              <div ref={desktopMenuRootRef} className="relative min-w-0">
                {hasDesktopSubmenu ? (
                  <div
                    ref={desktopSubmenuRef}
                    style={{
                      top: desktopSubmenuTop,
                      width: desktopSubmenuWidth,
                    } as React.CSSProperties}
                    className={cn(
                      "absolute flex max-h-[calc(100dvh-3rem)] flex-col overflow-hidden rounded-xl border-[0.5px] border-border bg-popover p-1.5 shadow-xs",
                      desktopSubmenuSide === "right" ? "left-[calc(100%+0.5rem)]" : "right-[calc(100%+0.5rem)]",
                    )}
                  >
                    <ModelMenuScrollContainer maxHeight={desktopSubmenuListMaxHeight}>
                      {reasoningActive && reasoning ? (
                        <ReasoningMenuItems
                          reasoning={reasoning}
                          formatLabel={formatReasoningLabel}
                          onSelect={(value) => {
                            reasoning.onChange(value);
                            closeMenu();
                          }}
                        />
                      ) : (
                        <div className="flex flex-col gap-0.5">
                          {activeDesktopGroup?.items.map((item) => (
                            <ChatModelMenuItem
                              key={item.platformModelName}
                              model={item}
                              selected={item.platformModelName === selectedPlatformModelName}
                              buttonRef={item.platformModelName === selectedPlatformModelName ? selectedModelButtonRef : undefined}
                              onSelect={() => {
                                onModelChange(item.platformModelName);
                                closeMenu();
                              }}
                            />
                          ))}
                        </div>
                      )}
                    </ModelMenuScrollContainer>
                  </div>
                ) : null}

                <div
                  ref={desktopGroupMenuRef}
                  className="flex min-w-0 max-h-[calc(100dvh-3rem)] flex-col overflow-hidden rounded-xl border-[0.5px] border-border bg-popover p-1.5 shadow-xs"
                >
                  <div className="flex h-7 shrink-0 items-center justify-between gap-3 px-2">
                    <span className="text-[11px] font-medium text-foreground">{t("group")}</span>
                    <span className="truncate text-[10px] font-medium text-muted-foreground">
                      {selectedGroupLabel}
                    </span>
                  </div>
                  {modelGroups.length === 0 ? (
                    <div className="px-2 py-3 text-[11px] leading-4 text-muted-foreground">
                      {t("empty")}
                    </div>
                  ) : (
                    <div className="min-h-0 min-w-0">
                      <ModelMenuScrollContainer maxHeight={desktopGroupListMaxHeight} onScroll={updateDesktopSubmenuMetrics}>
                        <div className="flex flex-col gap-0.5">
                          {modelGroups.map((group) => {
                            const selectedGroup = group.key === selectedGroupKey;
                            const activeGroup = !reasoningActive && group.key === activeDesktopGroup?.key;
                            const groupIconURL = resolveModelIconURL(group.icon);
                            return (
                              <button
                                type="button"
                                key={group.key}
                                ref={(node) => {
                                  if (node) {
                                    desktopGroupItemRefs.current.set(group.key, node);
                                    return;
                                  }
                                  desktopGroupItemRefs.current.delete(group.key);
                                }}
                                className={cn(
                                  "flex h-7 w-full items-center gap-2 rounded-md px-2 py-0 text-left text-[11px] font-medium outline-none transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:bg-accent focus-visible:text-accent-foreground",
                                  activeGroup ? "bg-accent text-accent-foreground" : "text-muted-foreground",
                                  selectedGroup && !activeGroup ? "text-foreground" : null,
                                )}
                                onMouseEnter={() => selectDesktopGroup(group.key)}
                                onFocus={() => selectDesktopGroup(group.key)}
                                onClick={() => selectDesktopGroup(group.key)}
                              >
                                <ModelIcon iconUrl={groupIconURL} label={group.label} />
                                <span className="min-w-0 flex-1 truncate font-medium">{group.label}</span>
                                <span className="shrink-0 text-[10px] tabular-nums text-muted-foreground/80">
                                  {group.items.length}
                                </span>
                                <ChevronRight className="size-3.5 shrink-0 text-muted-foreground/65" strokeWidth={1.8} />
                              </button>
                            );
                          })}
                        </div>
                      </ModelMenuScrollContainer>
                    </div>
                  )}
                  {reasoningAvailable ? (
                    <div className="mt-1 shrink-0 border-t border-border/60 pt-1">
                      <button
                        ref={reasoningButtonRef}
                        type="button"
                        className={cn(
                          "flex h-7 w-full items-center gap-2 rounded-md px-2 text-left text-[11px] font-medium text-muted-foreground outline-none transition-colors hover:bg-accent hover:text-accent-foreground focus-visible:bg-accent focus-visible:text-accent-foreground disabled:pointer-events-none disabled:opacity-50",
                          reasoningActive && "bg-accent text-accent-foreground",
                        )}
                        disabled={reasoning?.disabled}
                        onMouseEnter={() => setActiveGroupKey(REASONING_MENU_KEY)}
                        onFocus={() => setActiveGroupKey(REASONING_MENU_KEY)}
                        onClick={() => setActiveGroupKey(REASONING_MENU_KEY)}
                      >
                        <Brain className="size-3.5 shrink-0" strokeWidth={1.7} />
                        <span className="min-w-0 flex-1 truncate">{t("reasoning")}</span>
                        <span className="shrink-0 text-[10px] text-muted-foreground/80">{reasoningValueLabel}</span>
                        <ChevronRight className="size-3.5 shrink-0 text-muted-foreground/65" strokeWidth={1.8} />
                      </button>
                    </div>
                  ) : null}
                </div>
              </div>
            )}
        </PopoverContent>
      </Popover>
      </div>
    </>
  );
}
