import { describe, expect, it } from "vitest";
import { Call } from "@wailsio/runtime";

import { nonNull, parseAppError } from ".";

function runtimeError(cause: unknown): Call.RuntimeError {
  const error = new Call.RuntimeError("failed");
  error.cause = cause;
  return error;
}

describe("nonNull", () => {
  it.each([
    { name: "null", value: null },
    { name: "undefined", value: undefined },
    { name: "empty", value: [] },
  ])("$nameを空配列へ正規化する", ({ value }) => expect(nonNull(value)).toEqual([]));
});

describe("parseAppError", () => {
  it.each([
    {
      name: "structured cause",
      error: runtimeError({
        code: "validation_failed",
        message: "入力を確認してください",
        retryable: false,
      }),
      want: { code: "validation_failed", message: "入力を確認してください", retryable: false },
    },
    { name: "invalid cause", error: runtimeError({}), want: null },
    { name: "non runtime error", error: new Error("failed"), want: null },
  ])("$nameを変換する", ({ error, want }) => expect(parseAppError(error)).toEqual(want));
});
