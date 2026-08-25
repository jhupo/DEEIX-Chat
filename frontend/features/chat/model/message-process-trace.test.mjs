import assert from "node:assert/strict";
import test from "node:test";

import { parseStructuredTraceStages } from "./message-process-trace.ts";

const labels = {
  fileContext: {
    ready: (detail) => `ready: ${detail}`,
    includedDetail: (count) => `${count} included`,
    includedSummary: (count) => `${count} included`,
    skipped: (count) => `${count} skipped`,
    separator: ", ",
  },
  rag: {
    completed: (files, chunks) => `${files} files, ${chunks} chunks`,
    emptyWithFullText: "empty, full text",
    emptyNoFullText: "empty",
    lowScoreWithFullText: "low score, full text",
    lowScoreNoFullText: "low score",
    skippedFallback: "skipped",
    incompleteWithFullText: "incomplete, full text",
    incompleteNoFullText: "incomplete",
  },
};

test("renders Skill, attachment, file, and retrieval stages from one structured payload", () => {
  const stages = parseStructuredTraceStages(JSON.stringify({
    trace_stages: [
      {
        kind: "skill_context",
        status: "completed",
        skill_titles: ["Image editing"],
      },
      {
        kind: "mcp_attachment_processor",
        label: "Image analyzer",
        status: "completed",
        file_names: ["source.png"],
      },
      {
        kind: "file_context",
        status: "ready",
        included_count: 1,
        skipped_count: 0,
      },
      {
        kind: "content_retrieval",
        status: "completed",
        file_count: 1,
        chunk_count: 3,
      },
    ],
  }), labels);

  assert.deepEqual(stages.map(({ kind, label, status, details }) => ({ kind, label, status, details })), [
    {
      kind: "skill_context",
      label: "skill_context",
      status: "completed",
      details: ["Image editing"],
    },
    {
      kind: "mcp_attachment_processor",
      label: "Image analyzer",
      status: "completed",
      details: ["source.png"],
    },
    {
      kind: "file_context",
      label: "file_context",
      status: "ready",
      details: ["ready: 1 included"],
    },
    {
      kind: "content_retrieval",
      label: "content_retrieval",
      status: "completed",
      details: ["1 files, 3 chunks"],
    },
  ]);
});
