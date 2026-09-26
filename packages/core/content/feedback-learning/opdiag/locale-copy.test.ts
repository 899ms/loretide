// @vitest-environment node

import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const LOCALES = ["en", "zh-Hans", "ja", "ko"] as const;
const forbidden = /增长|提升|下降|导致|带来|因为|growth|caused|because/i;

function json(locale: string, name: "common" | "layout") {
  const path = resolve(__dirname, "../../../../views/locales", locale, `${name}.json`);
  return JSON.parse(readFileSync(path, "utf8")) as Record<string, unknown>;
}

function strings(value: unknown, path = ""): Array<{ path: string; value: string }> {
  if (typeof value === "string") return [{ path, value }];
  if (!value || typeof value !== "object") return [];
  return Object.entries(value).flatMap(([key, item]) => strings(item, path ? `${path}.${key}` : key));
}

describe("operating diagnosis locale copy", () => {
  it("keeps four locales neutral and distinct from development diagnostics", () => {
    for (const locale of LOCALES) {
      const common = json(locale, "common");
      const opdiag = common.contentOperatingDiagnosis;
      const details = common.contentOperatingDiagnosisDetails;
      const layout = json(locale, "layout") as { nav?: Record<string, string> };
      expect(opdiag).toBeTruthy();
      expect(details).toBeTruthy();
      for (const entry of [...strings(opdiag, "core"), ...strings(details, "details")]) {
        if (entry.path === "core.rulesMap.performance.difference_is_not_cause" || entry.path === "details.ruleMap.performance.difference_is_not_cause") continue;
        expect(entry.value).not.toMatch(forbidden);
      }
      expect(layout.nav?.operating_diagnosis).toBeTruthy();
      expect(layout.nav?.operating_diagnosis).not.toBe(layout.nav?.content_diagnostics);
    }
  });
});
