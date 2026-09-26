// @vitest-environment node
import { describe, expect, it } from "vitest";
import { opdiagNumberDisplay, opdiagReasonKey, opdiagRuleKey, opdiagSectionKey, opdiagStatusKey } from "./display";

describe("opdiag display", () => {
  it("keeps server display strings untouched", () => {
    expect(opdiagNumberDisplay({ status: "ok", value: "0", display: "0.00" })).toBe("0.00");
    expect(opdiagNumberDisplay({ status: "not_computable", reason: "no_data" })).toBeNull();
  });

  it("degrades unknown controlled values safely", () => {
    expect(opdiagStatusKey("unknown")).toBe("status.unknown");
    expect(opdiagReasonKey("future_reason")).toBe("reason.unknown");
    expect(opdiagSectionKey("future_section")).toBe("kind.unknown");
    expect(opdiagRuleKey("future.rule")).toBe("rule.default");
  });
});
