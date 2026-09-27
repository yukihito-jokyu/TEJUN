import { Call, Dialogs } from "@wailsio/runtime";
import { beforeEach, expect, it, vi } from "vitest";
import { prepareExportDestination } from "./ExportDestination";

vi.mock("@wailsio/runtime", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@wailsio/runtime")>();
  return { ...actual, Dialogs: { ...actual.Dialogs, OpenFile: vi.fn() } };
});

beforeEach(() => vi.clearAllMocks());

function permissionError() {
  const error = new Call.RuntimeError("保存先にアクセスできません");
  error.cause = {
    code: "destination_permission_denied",
    message: "保存先フォルダにアクセスできません",
    retryable: true,
  };
  return error;
}

it("保存先フォルダの許可後に同じ出力先を再検証する", async () => {
  const prepare = vi.fn().mockRejectedValueOnce(permissionError()).mockResolvedValueOnce("ready");
  vi.mocked(Dialogs.OpenFile).mockResolvedValue("/Users/example/Downloads");

  await expect(
    prepareExportDestination("/Users/example/Downloads/procedure.pdf", prepare),
  ).resolves.toBe("ready");
  expect(prepare).toHaveBeenCalledTimes(2);
  expect(Dialogs.OpenFile).toHaveBeenCalledWith(
    expect.objectContaining({
      Directory: "/Users/example/Downloads/",
      CanChooseDirectories: true,
      CanChooseFiles: false,
    }),
  );
});

it("フォルダ選択の取消時は出力を中止する", async () => {
  const prepare = vi.fn().mockRejectedValue(permissionError());
  vi.mocked(Dialogs.OpenFile).mockResolvedValue("");

  await expect(
    prepareExportDestination("/Users/example/Downloads/procedure.md", prepare),
  ).resolves.toBeNull();
  expect(prepare).toHaveBeenCalledTimes(1);
});

it("権限以外の失敗ではフォルダ選択を開かない", async () => {
  const error = new Error("保存先が変更されました");
  await expect(
    prepareExportDestination("/Users/example/Downloads/procedure.md", () => Promise.reject(error)),
  ).rejects.toBe(error);
  expect(Dialogs.OpenFile).not.toHaveBeenCalled();
});

it("Windowsのドライブ直下を保存先フォルダとして開く", async () => {
  const prepare = vi.fn().mockRejectedValueOnce(permissionError()).mockResolvedValueOnce("ready");
  vi.mocked(Dialogs.OpenFile).mockResolvedValue("C:\\");

  await expect(prepareExportDestination("C:\\procedure.pdf", prepare)).resolves.toBe("ready");
  expect(Dialogs.OpenFile).toHaveBeenCalledWith(expect.objectContaining({ Directory: "C:\\" }));
});

it("異なるフォルダを選んでも出力先を変えず、再検証エラーを返す", async () => {
  const denied = permissionError();
  const prepare = vi.fn().mockRejectedValueOnce(denied).mockRejectedValueOnce(denied);
  vi.mocked(Dialogs.OpenFile).mockResolvedValue("/Users/example/Desktop");

  await expect(
    prepareExportDestination("/Users/example/Downloads/procedure.md", prepare),
  ).rejects.toBe(denied);
  expect(prepare).toHaveBeenCalledTimes(2);
});
