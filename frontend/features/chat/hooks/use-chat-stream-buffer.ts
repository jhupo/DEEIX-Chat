"use client";

import * as React from "react";

import type { PendingExchangeMap } from "@/features/chat/types/chat-runtime";

const STREAM_TEXT_FLUSH_INTERVAL_MS = 100;

type StreamBuffer = {
  pendingText: string;
  textFrame: number | null;
  textTimeout: number | null;
  lastTextFlushAt: number;
};

function createStreamBuffer(): StreamBuffer {
  return {
    pendingText: "",
    textFrame: null,
    textTimeout: null,
    lastTextFlushAt: 0,
  };
}

function cancelBufferTimers(buffer: StreamBuffer) {
  if (buffer.textFrame !== null) window.cancelAnimationFrame(buffer.textFrame);
  if (buffer.textTimeout !== null) window.clearTimeout(buffer.textTimeout);
}

export function useChatStreamBuffer({
  setPendingExchanges,
}: {
  setPendingExchanges: React.Dispatch<React.SetStateAction<PendingExchangeMap>>;
}) {
  const buffersRef = React.useRef(new Map<string, StreamBuffer>());

  const flushStreamText = React.useCallback((exchangeKey: string) => {
    const buffer = buffersRef.current.get(exchangeKey);
    if (!buffer) return;
    buffer.textFrame = null;
    buffer.lastTextFlushAt = performance.now();
    const pendingText = buffer.pendingText;
    if (!pendingText) return;
    buffer.pendingText = "";

    setPendingExchanges((current) => {
      const exchange = current[exchangeKey];
      if (!exchange) return current;
      return {
        ...current,
        [exchangeKey]: {
          ...exchange,
          assistantPending: false,
          assistantStreaming: true,
          assistantText: exchange.assistantText + pendingText,
        },
      };
    });
  }, [setPendingExchanges]);

  const scheduleStreamFlush = React.useCallback((exchangeKey: string) => {
    const buffer = buffersRef.current.get(exchangeKey);
    if (!buffer || buffer.textFrame !== null || buffer.textTimeout !== null) return;
    const elapsed = performance.now() - buffer.lastTextFlushAt;
    if (elapsed >= STREAM_TEXT_FLUSH_INTERVAL_MS) {
      buffer.textFrame = window.requestAnimationFrame(() => flushStreamText(exchangeKey));
      return;
    }
    buffer.textTimeout = window.setTimeout(() => {
      buffer.textTimeout = null;
      buffer.textFrame = window.requestAnimationFrame(() => flushStreamText(exchangeKey));
    }, STREAM_TEXT_FLUSH_INTERVAL_MS - elapsed);
  }, [flushStreamText]);

  const enqueueStreamText = React.useCallback((exchangeKey: string, delta: string) => {
    const buffer = buffersRef.current.get(exchangeKey);
    if (!buffer || !delta) return;
    buffer.pendingText += delta;
    scheduleStreamFlush(exchangeKey);
  }, [scheduleStreamFlush]);

  const startStream = React.useCallback((exchangeKey: string) => {
    const existing = buffersRef.current.get(exchangeKey);
    if (existing) cancelBufferTimers(existing);
    buffersRef.current.set(exchangeKey, createStreamBuffer());
  }, []);

  const flushStreamTextNow = React.useCallback((exchangeKey: string) => {
    const buffer = buffersRef.current.get(exchangeKey);
    if (!buffer) return;
    if (buffer.textFrame !== null) {
      window.cancelAnimationFrame(buffer.textFrame);
      buffer.textFrame = null;
    }
    if (buffer.textTimeout !== null) {
      window.clearTimeout(buffer.textTimeout);
      buffer.textTimeout = null;
    }
    flushStreamText(exchangeKey);
  }, [flushStreamText]);

  const resetStreamBuffer = React.useCallback((exchangeKey?: string) => {
    if (exchangeKey) {
      const buffer = buffersRef.current.get(exchangeKey);
      if (!buffer) return;
      cancelBufferTimers(buffer);
      buffersRef.current.delete(exchangeKey);
      return;
    }
    for (const buffer of buffersRef.current.values()) cancelBufferTimers(buffer);
    buffersRef.current.clear();
  }, []);

  React.useEffect(() => () => resetStreamBuffer(), [resetStreamBuffer]);

  return {
    enqueueStreamText,
    flushStreamTextNow,
    resetStreamBuffer,
    startStream,
  };
}
