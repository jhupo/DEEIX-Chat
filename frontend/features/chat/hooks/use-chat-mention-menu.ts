"use client";

import * as React from "react";

import type { ConversationInputResourceDTO } from "@/shared/api/conversation.types";
import { listVisiblePromptPresets } from "@/shared/api/prompt-presets";
import type { PromptPresetDTO } from "@/shared/api/prompt-presets.types";
import { listConversationPlugins } from "@/shared/api/plugins";
import type { ConversationPluginDTO } from "@/shared/api/plugins";
import { listVisibleSkills } from "@/shared/api/skills";
import type { SkillSummaryDTO } from "@/shared/api/skills.types";
import { resolveAccessToken } from "@/shared/auth/resolve-access-token";

const MENTION_MENU_MAX_HEIGHT = 280;
const MENTION_MENU_MIN_HEIGHT = 32;
const MENTION_MENU_ROW_HEIGHT = 32;
const MENTION_MENU_ROW_GAP = 2;
const MENTION_MENU_SECTION_HEADER_HEIGHT = 28;
const MENTION_MENU_SECTION_GAP = 2;
const MENTION_MENU_CHROME_HEIGHT = 12;
const MENTION_MENU_VIEWPORT_GUTTER = 16;
const MENTION_MENU_OFFSET = 8;
const MENTION_MENU_QUERY_DELAY_MS = 180;
const DEFAULT_MENTION_MENU_KINDS: readonly ChatMentionMenuKind[] = ["skill", "plugin", "prompt"];

export type ChatMentionMenuKind = "skill" | "plugin" | "prompt";

type ChatMentionSkillMenuItem = {
  id: string;
  kind: "skill";
  label: string;
  description: string;
  skill: SkillSummaryDTO;
  selected: boolean;
};

type ChatMentionInputResourceMenuItem = {
  id: string;
  kind: "skill" | "plugin";
  label: string;
  description: string;
  resource: ConversationInputResourceDTO;
  selected: boolean;
};

type ChatMentionPromptMenuItem = {
  id: string;
  kind: "prompt";
  label: string;
  description: string;
  prompt: PromptPresetDTO;
  selected: false;
};

export type ChatMentionMenuItem =
  | ChatMentionSkillMenuItem
  | ChatMentionInputResourceMenuItem
  | ChatMentionPromptMenuItem;

export type ChatMentionMenuSection = {
  kind: ChatMentionMenuKind;
  items: ChatMentionMenuItem[];
};

export type ChatMentionMenuLayout = {
  bottom?: number;
  height: number;
  left: number;
  placement: "bottom" | "top";
  top?: number;
  width: number;
};

type ChatMentionMenuPlacementPreference = "auto" | "bottom" | "top";
type ChatMentionMenuPlacementAnchor = "caret" | "container";

type ChatMentionMenuControllerArgs = {
  inputResources?: ConversationInputResourceDTO[];
  disabled: boolean;
  draft: string;
  maxSelectedSkills: number;
  selectedSkills?: SkillSummaryDTO[];
  selectedInputResources?: ConversationInputResourceDTO[];
  anchorRef: React.RefObject<HTMLElement | null>;
  textareaRef: React.RefObject<HTMLTextAreaElement | null>;
  onDraftChange: (value: string) => void;
  enabledKinds?: readonly ChatMentionMenuKind[];
  onSelectedSkillsChange?: (skills: SkillSummaryDTO[]) => void;
  onSelectedInputResourcesChange?: (resources: ConversationInputResourceDTO[]) => void;
  placementAnchor?: ChatMentionMenuPlacementAnchor;
  placementPreference?: ChatMentionMenuPlacementPreference;
  onSkillLimitReached?: () => void;
};

type ChatMentionTriggerQuery = {
  kind: ChatMentionMenuKind;
  query: string;
  range: {
    start: number;
    end: number;
  };
};

type ChatMentionMenuAnchor = {
  height: number;
  left: number;
  top: number;
  width: number;
};

type ChatMentionSelection = {
  end: number;
  start: number;
};

function canStartTrigger(value: string, triggerIndex: number, trigger: "@" | "/"): boolean {
  if (triggerIndex === 0) {
    return true;
  }

  const previous = value[triggerIndex - 1] ?? "";
  if (/\s/.test(previous) || /[\u3400-\u9fff]/.test(previous)) {
    return true;
  }
  if (/[[({<，。！？、：；,.!?;:]/.test(previous)) {
    return true;
  }
  if (trigger === "@") {
    return !/[A-Za-z0-9._-]/.test(previous);
  }
  return !/[A-Za-z0-9._~:/?#@!$&'()*+,;=%-]/.test(previous);
}

export function resolveTriggerQuery(value: string, caretIndex: number): ChatMentionTriggerQuery | null {
  const end = Math.min(Math.max(caretIndex, 0), value.length);
  const prefix = value.slice(0, end);
  const mentionIndex = prefix.lastIndexOf("@");
  const promptIndex = prefix.lastIndexOf("/");
  const triggerIndex = Math.max(mentionIndex, promptIndex);
  const trigger = triggerIndex >= 0 ? prefix[triggerIndex] : "";
  if (trigger !== "@" && trigger !== "/") {
    return null;
  }
  if (!canStartTrigger(value, triggerIndex, trigger)) {
    return null;
  }

  const query = prefix.slice(triggerIndex + 1);
  if (/\s/.test(query)) {
    return null;
  }

  return {
    kind: trigger === "@" ? "skill" : "prompt",
    query: query.toLowerCase(),
    range: { start: triggerIndex, end },
  };
}

function readTextareaSelection(textarea: HTMLTextAreaElement | null, fallback: number): ChatMentionSelection {
  if (!textarea) {
    return { start: fallback, end: fallback };
  }
  return {
    start: textarea.selectionStart,
    end: textarea.selectionEnd,
  };
}

function createTextareaCaretMirror(textarea: HTMLTextAreaElement) {
  const styles = window.getComputedStyle(textarea);
  const mirror = document.createElement("div");
  mirror.style.position = "absolute";
  mirror.style.visibility = "hidden";
  mirror.style.pointerEvents = "none";
  mirror.style.whiteSpace = "pre-wrap";
  mirror.style.overflowWrap = "break-word";
  mirror.style.boxSizing = styles.boxSizing;
  mirror.style.width = styles.width;
  mirror.style.padding = styles.padding;
  mirror.style.border = styles.border;
  mirror.style.font = styles.font;
  mirror.style.fontFamily = styles.fontFamily;
  mirror.style.fontSize = styles.fontSize;
  mirror.style.fontWeight = styles.fontWeight;
  mirror.style.letterSpacing = styles.letterSpacing;
  mirror.style.lineHeight = styles.lineHeight;
  mirror.style.tabSize = styles.tabSize;
  mirror.style.textTransform = styles.textTransform;
  return mirror;
}

function resolveTextareaCaretAnchor(
  textarea: HTMLTextAreaElement | null,
  fallbackAnchor: HTMLElement,
  caretIndex: number,
): ChatMentionMenuAnchor {
  const fallbackRect = fallbackAnchor.getBoundingClientRect();
  if (!textarea || typeof document === "undefined") {
    return fallbackRect;
  }

  const textareaRect = textarea.getBoundingClientRect();
  if (textareaRect.width <= 0 || textareaRect.height <= 0) {
    return fallbackRect;
  }

  const mirror = createTextareaCaretMirror(textarea);
  const textBeforeCaret = textarea.value.slice(0, caretIndex);
  mirror.textContent = textBeforeCaret;
  const marker = document.createElement("span");
  marker.textContent = "\u200b";
  mirror.appendChild(marker);
  document.body.appendChild(mirror);

  const markerRect = marker.getBoundingClientRect();
  const styles = window.getComputedStyle(textarea);
  const borderTop = Number.parseFloat(styles.borderTopWidth) || 0;
  const mirrorRect = mirror.getBoundingClientRect();
  const markerTop = textareaRect.top + markerRect.top - mirrorRect.top - textarea.scrollTop - borderTop;
  const lineHeight = Number.parseFloat(styles.lineHeight) || textareaRect.height;
  document.body.removeChild(mirror);

  return {
    height: Math.max(1, lineHeight),
    left: fallbackRect.left,
    top: Math.min(Math.max(markerTop, textareaRect.top), textareaRect.bottom),
    width: fallbackRect.width,
  };
}

function resolveContainerAnchor(anchor: HTMLElement): ChatMentionMenuAnchor {
  const rect = anchor.getBoundingClientRect();
  return {
    height: rect.height,
    left: rect.left,
    top: rect.top,
    width: rect.width,
  };
}

function removeTriggerRange(value: string, range: ChatMentionTriggerQuery["range"]): {
  caretIndex: number;
  value: string;
} {
  const trailingSpace = value[range.end] === " " ? 1 : 0;
  return {
    caretIndex: range.start,
    value: `${value.slice(0, range.start)}${value.slice(range.end + trailingSpace)}`,
  };
}

function replaceTriggerRange(value: string, range: ChatMentionTriggerQuery["range"], replacement: string): {
  caretIndex: number;
  value: string;
} {
  const before = value.slice(0, range.start);
  const trailingSpace = value[range.end] === " " ? 1 : 0;
  const after = value.slice(range.end + trailingSpace);
  const prefix = before && replacement && !/\s$/.test(before) ? " " : "";
  const suffix = after && replacement && !/^\s/.test(after) ? " " : "";
  const inserted = `${prefix}${replacement}${suffix}`;
  return {
    caretIndex: before.length + prefix.length + replacement.length,
    value: `${before}${inserted}${after}`,
  };
}

function itemMatchesQuery(values: Array<string | undefined>, query: string): boolean {
  const normalizedQuery = query.trim().toLowerCase();
  if (!normalizedQuery) {
    return true;
  }
  return values.join(" ").toLowerCase().includes(normalizedQuery);
}

function skillsToItems(skills: SkillSummaryDTO[], selectedSkills: SkillSummaryDTO[]): ChatMentionSkillMenuItem[] {
  const selectedIDs = new Set(selectedSkills.map((skill) => skill.id));
  return skills.map((skill) => ({
    id: `skill:${skill.id}`,
    kind: "skill" as const,
    label: skill.trigger || skill.title,
    description: skill.description,
    skill,
    selected: selectedIDs.has(skill.id),
  }));
}

function promptsToItems(prompts: PromptPresetDTO[]): ChatMentionPromptMenuItem[] {
  return prompts.map((prompt) => ({
    id: `prompt:${prompt.scope}:${prompt.id}`,
    kind: "prompt" as const,
    label: prompt.trigger || prompt.title,
    description: prompt.description,
    prompt,
    selected: false,
  }));
}

function inputResourcesToItems(
  resources: ConversationInputResourceDTO[],
  query: string,
  selectedResources: ConversationInputResourceDTO[],
): ChatMentionInputResourceMenuItem[] {
  const selectedRefs = new Set(selectedResources.map((item) => item.resourceRef));
  return resources
    .filter((item) => itemMatchesQuery([item.name, item.description], query))
    .map((resource) => ({
      id: `resource:${resource.resourceRef}`,
      kind: resource.kind === "app-mention" ? "plugin" as const : "skill" as const,
      label: resource.name,
      description: resource.description,
      resource,
      selected: selectedRefs.has(resource.resourceRef),
    }));
}

function pluginsToItems(
  plugins: ConversationPluginDTO[],
  query: string,
  selectedResources: ConversationInputResourceDTO[],
): ChatMentionInputResourceMenuItem[] {
  return inputResourcesToItems(
    plugins.map((plugin) => ({
      kind: "app-mention" as const,
      name: plugin.key,
      description: plugin.description,
      resourceRef: plugin.resourceRef,
    })),
    query,
    selectedResources,
  );
}

function buildSections({
  inputResources,
  plugins,
  pluginLoading,
  prompts,
  promptLoading,
  skillLoading,
  skills,
  query,
  queryKind,
  selectedSkills = [],
  selectedInputResources = [],
  enabledKinds,
}: {
  inputResources?: ConversationInputResourceDTO[];
  plugins: ConversationPluginDTO[];
  pluginLoading: boolean;
  prompts: PromptPresetDTO[];
  promptLoading: boolean;
  skills: SkillSummaryDTO[];
  skillLoading: boolean;
  query: string | null;
  queryKind: ChatMentionMenuKind | null;
  selectedSkills: SkillSummaryDTO[];
  selectedInputResources: ConversationInputResourceDTO[];
  enabledKinds: ReadonlySet<ChatMentionMenuKind>;
}): ChatMentionMenuSection[] {
  if (query === null) {
    return [];
  }

  if (queryKind === "skill" && (enabledKinds.has("skill") || enabledKinds.has("plugin"))) {
    if (inputResources !== undefined) {
      const items = inputResourcesToItems(inputResources, query, selectedInputResources);
      return (["skill", "plugin"] as const).flatMap((kind) => {
        if (!enabledKinds.has(kind)) return [];
        const sectionItems = items.filter((item) => item.kind === kind);
        return sectionItems.length > 0 ? [{ kind, items: sectionItems }] : [];
      });
    }
    const sections: ChatMentionMenuSection[] = [];
    const skillItems = skillLoading || !enabledKinds.has("skill") ? [] : skillsToItems(skills, selectedSkills);
    const pluginItems = pluginLoading || !enabledKinds.has("plugin")
      ? []
      : pluginsToItems(plugins, query, selectedInputResources);
    if (skillItems.length > 0) sections.push({ kind: "skill", items: skillItems });
    if (pluginItems.length > 0) sections.push({ kind: "plugin", items: pluginItems });
    return sections;
  }

  if (queryKind === "prompt" && enabledKinds.has("prompt")) {
    const items = promptLoading ? [] : promptsToItems(prompts);
    return items.length > 0 ? [{ kind: "prompt", items }] : [];
  }

  return [];
}

function flattenSections(sections: ChatMentionMenuSection[]): ChatMentionMenuItem[] {
  return sections.flatMap((section) => section.items);
}

function resolveMentionMenuWidth(anchorWidth: number, viewportWidth: number): number {
  const availableWidth = Math.max(0, viewportWidth - MENTION_MENU_VIEWPORT_GUTTER * 2);
  return Math.min(anchorWidth, availableWidth);
}

function resolveMentionMenuContentHeight(sections: ChatMentionMenuSection[]): number {
  const itemCount = sections.reduce((total, section) => total + section.items.length, 0);
  if (itemCount === 0) {
    return MENTION_MENU_MIN_HEIGHT;
  }
  const sectionChrome = sections.length * MENTION_MENU_SECTION_HEADER_HEIGHT;
  const sectionGaps = sections.length * MENTION_MENU_SECTION_GAP;
  return Math.min(
    MENTION_MENU_MAX_HEIGHT,
    itemCount * MENTION_MENU_ROW_HEIGHT
      + Math.max(0, itemCount - 1) * MENTION_MENU_ROW_GAP
      + sectionChrome
      + sectionGaps
      + MENTION_MENU_CHROME_HEIGHT,
  );
}

function resolveMentionMenuLayout(
  anchor: ChatMentionMenuAnchor,
  sections: ChatMentionMenuSection[],
  viewportWidth: number,
  viewportHeight: number,
  placementPreference: ChatMentionMenuPlacementPreference,
): ChatMentionMenuLayout {
  const preferredTop = anchor.top + anchor.height + MENTION_MENU_OFFSET;
  const preferredBottom = anchor.top - MENTION_MENU_OFFSET;
  const desiredHeight = resolveMentionMenuContentHeight(sections);
  const availableBelow = viewportHeight - preferredTop - MENTION_MENU_VIEWPORT_GUTTER;
  const availableAbove = preferredBottom - MENTION_MENU_VIEWPORT_GUTTER;
  const anchorInLowerHalf = anchor.top + anchor.height / 2 > viewportHeight / 2;
  const hasUsableAbove = availableAbove >= Math.min(desiredHeight, MENTION_MENU_MIN_HEIGHT);
  const openBelow =
    placementPreference === "bottom" ||
    (placementPreference === "top"
      ? !hasUsableAbove
      : !anchorInLowerHalf ||
        availableBelow >= Math.min(desiredHeight, MENTION_MENU_MIN_HEIGHT) ||
        availableBelow >= availableAbove);
  const availableHeight = Math.max(0, openBelow ? availableBelow : availableAbove);
  const maxHeight = Math.max(
    Math.min(MENTION_MENU_MIN_HEIGHT, availableHeight),
    Math.min(desiredHeight, availableHeight),
  );
  const preferredWidth = resolveMentionMenuWidth(anchor.width, viewportWidth);
  const preferredLeft = anchor.left;
  const maxLeft = Math.max(
    MENTION_MENU_VIEWPORT_GUTTER,
    viewportWidth - preferredWidth - MENTION_MENU_VIEWPORT_GUTTER,
  );
  const left = Math.min(Math.max(preferredLeft, MENTION_MENU_VIEWPORT_GUTTER), maxLeft);
  const width = Math.min(
    preferredWidth,
    Math.max(0, viewportWidth - left - MENTION_MENU_VIEWPORT_GUTTER),
  );

  if (openBelow) {
    return { height: maxHeight, left, placement: "bottom", top: preferredTop, width };
  }

  return {
    bottom: Math.max(MENTION_MENU_VIEWPORT_GUTTER, viewportHeight - preferredBottom),
    height: maxHeight,
    left,
    placement: "top",
    width,
  };
}

function mentionMenuLayoutsEqual(
  previous: ChatMentionMenuLayout | null,
  next: ChatMentionMenuLayout,
): boolean {
  return Boolean(
    previous &&
      previous.bottom === next.bottom &&
      previous.height === next.height &&
      previous.left === next.left &&
      previous.placement === next.placement &&
      previous.top === next.top &&
      previous.width === next.width,
  );
}

export function useChatMentionMenu({
  inputResources,
  disabled,
  draft,
  maxSelectedSkills,
  selectedSkills = [],
  selectedInputResources = [],
  anchorRef,
  textareaRef,
  onDraftChange,
  onSelectedSkillsChange,
  onSelectedInputResourcesChange,
  enabledKinds = DEFAULT_MENTION_MENU_KINDS,
  placementAnchor = "caret",
  placementPreference = "auto",
  onSkillLimitReached,
}: ChatMentionMenuControllerArgs) {
  const menuRef = React.useRef<HTMLDivElement | null>(null);
  const menuID = React.useId();
  const [inputFocused, setInputFocused] = React.useState(false);
  const [activeIndex, setActiveIndex] = React.useState(0);
  const [dismissedTriggerKey, setDismissedTriggerKey] = React.useState<string | null>(null);
  const [menuLayout, setMenuLayout] = React.useState<ChatMentionMenuLayout | null>(null);
  const [skills, setSkills] = React.useState<SkillSummaryDTO[]>([]);
  const [skillsLoading, setSkillsLoading] = React.useState(false);
  const [plugins, setPlugins] = React.useState<ConversationPluginDTO[]>([]);
  const [pluginsLoading, setPluginsLoading] = React.useState(false);
  const [prompts, setPrompts] = React.useState<PromptPresetDTO[]>([]);
  const [promptsLoading, setPromptsLoading] = React.useState(false);
  const [selection, setSelection] = React.useState<ChatMentionSelection>(() => ({
    end: draft.length,
    start: draft.length,
  }));
  const enabledKindSet = React.useMemo(() => new Set(enabledKinds), [enabledKinds]);
  const triggerQuery = selection.start === selection.end ? resolveTriggerQuery(draft, selection.start) : null;
  const query = triggerQuery?.query ?? null;
  const queryKind = triggerQuery?.kind ?? null;
  const skillQuery = queryKind === "skill" ? query : null;
  const promptQuery = queryKind === "prompt" ? query : null;
  const triggerKey = triggerQuery
    ? `${draft}:${triggerQuery.kind}:${triggerQuery.range.start}:${triggerQuery.range.end}:${triggerQuery.query}`
    : null;

  const updateSelection = React.useCallback(() => {
    const nextSelection = readTextareaSelection(textareaRef.current, draft.length);
    setSelection((currentSelection) => (
      currentSelection.start === nextSelection.start && currentSelection.end === nextSelection.end
        ? currentSelection
        : nextSelection
    ));
  }, [draft.length, textareaRef]);

  React.useLayoutEffect(() => {
    updateSelection();
  }, [draft, updateSelection]);

  React.useEffect(() => {
    if (inputResources !== undefined || skillQuery === null || disabled || !enabledKindSet.has("skill")) {
      setSkills([]);
      setSkillsLoading(false);
      return;
    }

    const controller = new AbortController();
    const timer = window.setTimeout(() => {
      setSkillsLoading(true);
      void (async () => {
        try {
          const token = await resolveAccessToken();
          if (!token || controller.signal.aborted) {
            return;
          }
          const data = await listVisibleSkills(token, { query: skillQuery, page: 1, pageSize: 50 });
          if (!controller.signal.aborted) {
            setSkills(data.results);
          }
        } catch {
          if (!controller.signal.aborted) {
            setSkills([]);
          }
        } finally {
          if (!controller.signal.aborted) {
            setSkillsLoading(false);
          }
        }
      })();
    }, MENTION_MENU_QUERY_DELAY_MS);

    return () => {
      controller.abort();
      window.clearTimeout(timer);
    };
  }, [disabled, enabledKindSet, inputResources, skillQuery]);

  React.useEffect(() => {
    if (inputResources !== undefined || skillQuery === null || disabled || !enabledKindSet.has("plugin")) {
      setPlugins([]);
      setPluginsLoading(false);
      return;
    }

    const controller = new AbortController();
    const timer = window.setTimeout(() => {
      setPluginsLoading(true);
      void (async () => {
        try {
          const token = await resolveAccessToken();
          if (!token || controller.signal.aborted) return;
          const data = await listConversationPlugins(token);
          if (!controller.signal.aborted) setPlugins(data);
        } catch {
          if (!controller.signal.aborted) setPlugins([]);
        } finally {
          if (!controller.signal.aborted) setPluginsLoading(false);
        }
      })();
    }, MENTION_MENU_QUERY_DELAY_MS);

    return () => {
      controller.abort();
      window.clearTimeout(timer);
    };
  }, [disabled, enabledKindSet, inputResources, skillQuery]);

  React.useEffect(() => {
    if (promptQuery === null || disabled || !enabledKindSet.has("prompt")) {
      setPrompts([]);
      setPromptsLoading(false);
      return;
    }

    const controller = new AbortController();
    const timer = window.setTimeout(() => {
      setPromptsLoading(true);
      void (async () => {
        try {
          const token = await resolveAccessToken();
          if (!token || controller.signal.aborted) {
            return;
          }
          const data = await listVisiblePromptPresets(token, {
            query: promptQuery,
            enabled: true,
            page: 1,
            pageSize: 50,
          });
          if (!controller.signal.aborted) {
            setPrompts(data.results);
          }
        } catch {
          if (!controller.signal.aborted) {
            setPrompts([]);
          }
        } finally {
          if (!controller.signal.aborted) {
            setPromptsLoading(false);
          }
        }
      })();
    }, MENTION_MENU_QUERY_DELAY_MS);

    return () => {
      controller.abort();
      window.clearTimeout(timer);
    };
  }, [disabled, enabledKindSet, promptQuery]);

  const sections = React.useMemo(
    () =>
      buildSections({
        inputResources,
        plugins,
        pluginLoading: pluginsLoading,
        prompts,
        promptLoading: promptsLoading,
        skills,
        skillLoading: skillsLoading,
        query,
        queryKind,
        selectedSkills,
        selectedInputResources,
        enabledKinds: enabledKindSet,
      }),
    [
      inputResources,
      plugins,
      pluginsLoading,
      prompts,
      promptsLoading,
      skills,
      skillsLoading,
      query,
      queryKind,
      selectedSkills,
      selectedInputResources,
      enabledKindSet,
    ],
  );
  const items = React.useMemo(() => flattenSections(sections), [sections]);
  const open = inputFocused && query !== null && dismissedTriggerKey !== triggerKey && !disabled && items.length > 0;
  const activeItem = open ? items[Math.min(activeIndex, items.length - 1)] : null;

  React.useEffect(() => {
    setActiveIndex(0);
  }, [query]);

  React.useEffect(() => {
    setActiveIndex((current) => (items.length === 0 ? 0 : Math.min(current, items.length - 1)));
  }, [items.length]);

  React.useEffect(() => {
    if (!open) {
      return;
    }
    const frameID = window.requestAnimationFrame(() => {
      const scrollContainer = menuRef.current?.querySelector<HTMLElement>("[data-mention-menu-scroll]");
      if (activeIndex === 0) {
        if (scrollContainer) {
          scrollContainer.scrollTop = 0;
        }
        return;
      }
      const activeElement = menuRef.current?.querySelector<HTMLElement>('[data-active="true"]');
      activeElement?.scrollIntoView({ block: "nearest" });
    });
    return () => window.cancelAnimationFrame(frameID);
  }, [activeIndex, open]);

  const updateLayout = React.useCallback(() => {
    if (!open || typeof window === "undefined") {
      return;
    }

    const anchor = anchorRef.current;
    if (!anchor) {
      return;
    }

    const menuAnchor =
      placementAnchor === "container"
        ? resolveContainerAnchor(anchor)
        : resolveTextareaCaretAnchor(textareaRef.current, anchor, triggerQuery?.range.start ?? draft.length);
    const nextLayout = resolveMentionMenuLayout(menuAnchor, sections, window.innerWidth, window.innerHeight, placementPreference);
    setMenuLayout((current) => (mentionMenuLayoutsEqual(current, nextLayout) ? current : nextLayout));
  }, [anchorRef, draft.length, open, placementAnchor, placementPreference, sections, textareaRef, triggerQuery?.range.start]);

  React.useLayoutEffect(() => {
    if (!open) {
      setMenuLayout(null);
      return;
    }
    updateLayout();
    let frameID = window.requestAnimationFrame(updateLayout);
    const update = () => {
      window.cancelAnimationFrame(frameID);
      frameID = window.requestAnimationFrame(updateLayout);
    };
    window.addEventListener("resize", update);
    window.addEventListener("scroll", update, true);
    return () => {
      window.cancelAnimationFrame(frameID);
      window.removeEventListener("resize", update);
      window.removeEventListener("scroll", update, true);
    };
  }, [open, updateLayout]);

  const focusTextarea = React.useCallback((caretIndex: number) => {
    window.requestAnimationFrame(() => {
      const textarea = textareaRef.current;
      textarea?.focus();
      textarea?.setSelectionRange(caretIndex, caretIndex);
    });
  }, [textareaRef]);

  const finishSelection = React.useCallback((replacement?: string) => {
    if (!triggerQuery) {
      return;
    }
    const nextDraft = replacement === undefined
      ? removeTriggerRange(draft, triggerQuery.range)
      : replaceTriggerRange(draft, triggerQuery.range, replacement);
    onDraftChange(nextDraft.value);
    setDismissedTriggerKey(null);
    focusTextarea(nextDraft.caretIndex);
  }, [draft, focusTextarea, onDraftChange, triggerQuery]);

  const select = React.useCallback(
    (item: ChatMentionMenuItem) => {
      if (item.kind === "prompt") {
        finishSelection(item.prompt.content.trim());
        return;
      }
      if ("resource" in item) {
        const alreadySelected = selectedInputResources.some(
          (resource) => resource.resourceRef === item.resource.resourceRef,
        );
        if (!alreadySelected && selectedInputResources.length >= maxSelectedSkills) {
          onSkillLimitReached?.();
          return;
        }
        onSelectedInputResourcesChange?.(
          alreadySelected
            ? selectedInputResources.filter((resource) => resource.resourceRef !== item.resource.resourceRef)
            : [...selectedInputResources, item.resource],
        );
        finishSelection();
        return;
      }
      if (item.kind === "skill") {
        const alreadySelected = selectedSkills.some((skill) => skill.id === item.skill.id);
        if (!alreadySelected && selectedSkills.length >= maxSelectedSkills) {
          onSkillLimitReached?.();
          return;
        }
        onSelectedSkillsChange?.(
          alreadySelected
            ? selectedSkills.filter((skill) => skill.id !== item.skill.id)
            : [...selectedSkills, item.skill],
        );
        finishSelection();
        return;
      }

    },
    [
      finishSelection,
      maxSelectedSkills,
      onSelectedSkillsChange,
      onSelectedInputResourcesChange,
      onSkillLimitReached,
      selectedSkills,
      selectedInputResources,
    ],
  );

  const handleChange = React.useCallback(
    (value: string) => {
      if (dismissedTriggerKey !== null) {
        setDismissedTriggerKey(null);
      }
      updateSelection();
      onDraftChange(value);
    },
    [dismissedTriggerKey, onDraftChange, updateSelection],
  );

  const handleSelectionChange = React.useCallback(() => {
    updateSelection();
  }, [updateSelection]);

  const handleKeyDown = React.useCallback(
    (event: React.KeyboardEvent<HTMLTextAreaElement>): boolean => {
      if (!open) {
        return false;
      }
      if (event.key === "ArrowDown") {
        event.preventDefault();
        setActiveIndex((current) => (current + 1) % items.length);
        return true;
      }
      if (event.key === "ArrowUp") {
        event.preventDefault();
        setActiveIndex((current) => (current - 1 + items.length) % items.length);
        return true;
      }
      if ((event.key === "Enter" || event.key === "Tab") && activeItem) {
        event.preventDefault();
        select(activeItem);
        return true;
      }
      if (event.key === "Escape") {
        event.preventDefault();
        setDismissedTriggerKey(triggerKey);
        return true;
      }
      return false;
    },
    [activeItem, items.length, open, select, triggerKey],
  );

  return {
    activeIndex,
    handleBlur: () => setInputFocused(false),
    handleChange,
    handleFocus: () => {
      setInputFocused(true);
      updateSelection();
    },
    handleKeyDown,
    handleSelectionChange,
    menuID,
    menuRef,
    menuLayout,
    menuReady: open && menuLayout !== null && menuLayout.height > 0 && menuLayout.width > 0,
    open,
    sections,
    select,
  };
}
