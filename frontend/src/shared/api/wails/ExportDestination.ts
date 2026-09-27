import { Dialogs } from "@wailsio/runtime";
import { parseAppError } from "./errors";

export async function prepareExportDestination<T>(
  path: string,
  prepare: () => Promise<T>,
): Promise<T | null> {
  try {
    return await prepare();
  } catch (error) {
    if (parseAppError(error)?.code !== "destination_permission_denied") throw error;

    const parent = path.slice(0, Math.max(path.lastIndexOf("/"), path.lastIndexOf("\\")) + 1);
    const chosen = await Dialogs.OpenFile({
      Title: "保存先フォルダへのアクセスを許可",
      Message: "先ほど保存先に選んだフォルダを選択してください",
      Directory: parent,
      CanChooseDirectories: true,
      CanChooseFiles: false,
    });
    if (!chosen) return null;
    return prepare();
  }
}
