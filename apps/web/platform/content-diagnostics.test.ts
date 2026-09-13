/**
 * @vitest-environment jsdom
 */
import { afterEach, describe, expect, it, vi } from "vitest";

import { downloadDiagnosticBundle } from "./content-diagnostics";

describe("downloadDiagnosticBundle", () => {
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
    vi.restoreAllMocks();
  });

  it("writes a readable JSON bundle before it starts the download", async () => {
    vi.useFakeTimers();
    let bundle: Blob | undefined;
    const createObjectURL = vi.fn((value: Blob) => {
      bundle = value;
      return "blob:diagnostics";
    });
    const revokeObjectURL = vi.fn();
    vi.stubGlobal("URL", { createObjectURL, revokeObjectURL });
    const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => undefined);
    const data = { run: { run_id: "run-1" }, redacted: true, source_text: undefined };

    downloadDiagnosticBundle(data);

    expect(click).toHaveBeenCalledOnce();
    expect(createObjectURL).toHaveBeenCalledOnce();
    expect(bundle?.type).toBe("application/json");
    expect(JSON.parse(await bundle!.text())).toEqual(data);

    vi.advanceTimersByTime(1000);
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:diagnostics");
  });
});
