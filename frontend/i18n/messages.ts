import type { AppLocale } from "@/i18n/config";
import enAdminAnnouncements from "@/i18n/messages/en-US/admin-announcements.json";
import enAdminContentModeration from "@/i18n/messages/en-US/admin-content-moderation.json";
import enAdminConversation from "@/i18n/messages/en-US/admin-conversation.json";
import enAdminFiles from "@/i18n/messages/en-US/admin-files.json";
import enAdminGroups from "@/i18n/messages/en-US/admin-groups.json";
import enAdminLogin from "@/i18n/messages/en-US/admin-login.json";
import enAdminLogs from "@/i18n/messages/en-US/admin-logs.json";
import enAdminModels from "@/i18n/messages/en-US/admin-models.json";
import enAdminPrompts from "@/i18n/messages/en-US/admin-prompts.json";
import enAdminRelays from "@/i18n/messages/en-US/admin-relays.json";
import enAdminTools from "@/i18n/messages/en-US/admin-tools.json";
import enAdminUpstreams from "@/i18n/messages/en-US/admin-upstreams.json";
import enAdminUsers from "@/i18n/messages/en-US/admin-users.json";
import enAnnouncements from "@/i18n/messages/en-US/announcements.json";
import enChat from "@/i18n/messages/en-US/chat.json";
import enCommon from "@/i18n/messages/en-US/common.json";
import enConversation from "@/i18n/messages/en-US/conversation.json";
import enErrors from "@/i18n/messages/en-US/errors.json";
import enFiles from "@/i18n/messages/en-US/files.json";
import enGuide from "@/i18n/messages/en-US/guide.json";
import enKnowledgeBases from "@/i18n/messages/en-US/knowledge-bases.json";
import enLogin from "@/i18n/messages/en-US/login.json";
import enPrompts from "@/i18n/messages/en-US/prompts.json";
import enRecent from "@/i18n/messages/en-US/recent.json";
import enSettings from "@/i18n/messages/en-US/settings.json";
import enShare from "@/i18n/messages/en-US/share.json";
import zhAdminAnnouncements from "@/i18n/messages/zh-CN/admin-announcements.json";
import zhAdminContentModeration from "@/i18n/messages/zh-CN/admin-content-moderation.json";
import zhAdminConversation from "@/i18n/messages/zh-CN/admin-conversation.json";
import zhAdminFiles from "@/i18n/messages/zh-CN/admin-files.json";
import zhAdminGroups from "@/i18n/messages/zh-CN/admin-groups.json";
import zhAdminLogin from "@/i18n/messages/zh-CN/admin-login.json";
import zhAdminLogs from "@/i18n/messages/zh-CN/admin-logs.json";
import zhAdminModels from "@/i18n/messages/zh-CN/admin-models.json";
import zhAdminPrompts from "@/i18n/messages/zh-CN/admin-prompts.json";
import zhAdminRelays from "@/i18n/messages/zh-CN/admin-relays.json";
import zhAdminTools from "@/i18n/messages/zh-CN/admin-tools.json";
import zhAdminUpstreams from "@/i18n/messages/zh-CN/admin-upstreams.json";
import zhAdminUsers from "@/i18n/messages/zh-CN/admin-users.json";
import zhAnnouncements from "@/i18n/messages/zh-CN/announcements.json";
import zhChat from "@/i18n/messages/zh-CN/chat.json";
import zhCommon from "@/i18n/messages/zh-CN/common.json";
import zhConversation from "@/i18n/messages/zh-CN/conversation.json";
import zhErrors from "@/i18n/messages/zh-CN/errors.json";
import zhFiles from "@/i18n/messages/zh-CN/files.json";
import zhGuide from "@/i18n/messages/zh-CN/guide.json";
import zhKnowledgeBases from "@/i18n/messages/zh-CN/knowledge-bases.json";
import zhLogin from "@/i18n/messages/zh-CN/login.json";
import zhPrompts from "@/i18n/messages/zh-CN/prompts.json";
import zhRecent from "@/i18n/messages/zh-CN/recent.json";
import zhSettings from "@/i18n/messages/zh-CN/settings.json";
import zhShare from "@/i18n/messages/zh-CN/share.json";
import { replaceDefaultBrandTitle } from "@/shared/config/branding";

const ENGLISH_MESSAGES = {
  common: enCommon,
  conversation: enConversation,
  errors: enErrors,
  login: enLogin,
  prompts: enPrompts,
  guide: enGuide,
  chat: enChat,
  announcements: enAnnouncements,
  recent: enRecent,
  share: enShare,
  files: enFiles,
  settings: enSettings,
  knowledgeBases: enKnowledgeBases,
  adminAnnouncements: enAdminAnnouncements,
  adminConversation: enAdminConversation,
  adminContentModeration: enAdminContentModeration,
  adminFiles: enAdminFiles,
  adminGroups: enAdminGroups,
  adminLogin: enAdminLogin,
  adminLogs: enAdminLogs,
  adminModels: enAdminModels,
  adminPrompts: enAdminPrompts,
  adminRelays: enAdminRelays,
  adminTools: enAdminTools,
  adminUpstreams: enAdminUpstreams,
  adminUsers: enAdminUsers,
};

const CHINESE_MESSAGES = {
  common: zhCommon,
  conversation: zhConversation,
  errors: zhErrors,
  login: zhLogin,
  prompts: zhPrompts,
  guide: zhGuide,
  chat: zhChat,
  announcements: zhAnnouncements,
  recent: zhRecent,
  share: zhShare,
  files: zhFiles,
  settings: zhSettings,
  knowledgeBases: zhKnowledgeBases,
  adminAnnouncements: zhAdminAnnouncements,
  adminConversation: zhAdminConversation,
  adminContentModeration: zhAdminContentModeration,
  adminFiles: zhAdminFiles,
  adminGroups: zhAdminGroups,
  adminLogin: zhAdminLogin,
  adminLogs: zhAdminLogs,
  adminModels: zhAdminModels,
  adminPrompts: zhAdminPrompts,
  adminRelays: zhAdminRelays,
  adminTools: zhAdminTools,
  adminUpstreams: zhAdminUpstreams,
  adminUsers: zhAdminUsers,
} satisfies typeof ENGLISH_MESSAGES;

export type AppMessages = typeof ENGLISH_MESSAGES;

export function applyBrandingToMessages(messages: AppMessages, brandTitle: string): AppMessages {
  return {
    ...messages,
    guide: {
      ...messages.guide,
      userWelcomeTitle: replaceDefaultBrandTitle(messages.guide.userWelcomeTitle, brandTitle),
    },
    recent: {
      ...messages.recent,
      allConversationsDescription: replaceDefaultBrandTitle(messages.recent.allConversationsDescription, brandTitle),
    },
    login: {
      ...messages.login,
      title: replaceDefaultBrandTitle(messages.login.title, brandTitle),
    },
    share: {
      ...messages.share,
      signInToContinue: replaceDefaultBrandTitle(messages.share.signInToContinue, brandTitle),
    },
    chat: {
      ...messages.chat,
      placeholder: replaceDefaultBrandTitle(messages.chat.placeholder, brandTitle),
    },
    settings: {
      ...messages.settings,
      accountPage: {
        ...messages.settings.accountPage,
        securityDialog: {
          ...messages.settings.accountPage.securityDialog,
          email: {
            ...messages.settings.accountPage.securityDialog.email,
            description: {
              ...messages.settings.accountPage.securityDialog.email.description,
              change: replaceDefaultBrandTitle(
                messages.settings.accountPage.securityDialog.email.description.change,
                brandTitle,
              ),
            },
          },
        },
      },
    },
  };
}

export const DEFAULT_MESSAGES: AppMessages = ENGLISH_MESSAGES;

export function loadLocaleMessages(locale: AppLocale): AppMessages {
  return locale === "zh-CN" ? CHINESE_MESSAGES : DEFAULT_MESSAGES;
}
