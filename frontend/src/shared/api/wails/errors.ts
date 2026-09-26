import { Call } from "@wailsio/runtime";
import { z } from "zod";

const appErrorCauseSchema = z
  .object({
    code: z.string(),
    message: z.string(),
    fieldErrors: z.record(z.string(), z.string()).optional(),
    retryable: z.boolean(),
    causeId: z.string().optional(),
    currentRevision: z.number().int().optional(),
  })
  .strict();

export type AppErrorCause = z.infer<typeof appErrorCauseSchema>;

export function parseAppError(error: unknown): AppErrorCause | null {
  if (!(error instanceof Call.RuntimeError)) return null;

  const result = appErrorCauseSchema.safeParse(error.cause);
  return result.success ? result.data : null;
}
