export function shouldSurfaceConversationLoadError(messageCount: number): boolean {
  return messageCount <= 0;
}

export function shouldRefreshMessagesAfterHistory(messageCount: number, historyWasLoaded: boolean): boolean {
  return messageCount <= 0 || !historyWasLoaded;
}
