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

  it("saves the server's own bytes under the name the server asked for", async () => {
    vi.useFakeTimers();
    let saved: Blob | undefined;
    const createObjectURL = vi.fn((value: Blob) => {
      saved = value;
      return "blob:diagnostics";
    });
    const revokeObjectURL = vi.fn();
    vi.stubGlobal("URL", { createObjectURL, revokeObjectURL });
    let downloadName: string | undefined;
    const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (this: HTMLAnchorElement) {
      downloadName = this.download;
    });
    // The wire format, not the camelCase shape the preview schema produces:
    // the bundle is read next to the server's own logs.
    const bytes = `{"run":{"run_id":"run-1"},"redacted":true}`;
    const blob = new Blob([bytes], { type: "application/json" });

    downloadDiagnosticBundle({ blob, filename: "loretide-diagnostics-run-1.json" });

    expect(click).toHaveBeenCalledOnce();
    expect(createObjectURL).toHaveBeenCalledOnce();
    expect(saved).toBe(blob);
    expect(await saved!.text()).toBe(bytes);
    expect(downloadName).toBe("loretide-diagnostics-run-1.json");

    vi.advanceTimersByTime(1000);
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:diagnostics");
  });
});
