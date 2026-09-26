export { parseAppError, type AppErrorCause } from "./errors";

export function nonNull<T>(items: T[] | null | undefined): T[] {
  return items ?? [];
}
