import { describe, expect, it } from "vitest";
import { incidentSeverityFromLabel, severityBucket } from "../src/lib/alertLabels.ts";

describe("severityBucket", () => {
  it("buckets free-form labels via substring matching", () => {
    expect(severityBucket("critical")).toBe("critical");
    expect(severityBucket("FATAL")).toBe("critical");
    expect(severityBucket("emergency")).toBe("critical");
    expect(severityBucket("error")).toBe("critical");
    expect(severityBucket("high")).toBe("high");
    expect(severityBucket("warning")).toBe("warning");
    expect(severityBucket("warn")).toBe("warning");
    expect(severityBucket("medium")).toBe("warning");
    expect(severityBucket("info")).toBe("default");
    expect(severityBucket("low")).toBe("default");
    expect(severityBucket(null)).toBe("default");
    expect(severityBucket(undefined)).toBe("default");
  });
});

describe("incidentSeverityFromLabel", () => {
  it("maps buckets onto the incident Severity enum", () => {
    expect(incidentSeverityFromLabel("critical")).toBe("critical");
    expect(incidentSeverityFromLabel("page (high)")).toBe("high");
    expect(incidentSeverityFromLabel("warn")).toBe("warning");
    expect(incidentSeverityFromLabel("info")).toBe("info");
    expect(incidentSeverityFromLabel(null)).toBe("info");
  });

  it("agrees with severityBucket for every bucket", () => {
    for (const label of ["critical", "fatal", "high", "warn", "medium", "info", "low", ""]) {
      const bucket = severityBucket(label);
      const mapped = incidentSeverityFromLabel(label);
      expect(mapped).toBe(bucket === "default" ? "info" : bucket);
    }
  });
});
