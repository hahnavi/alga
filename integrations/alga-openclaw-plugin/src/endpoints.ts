import { URL } from "node:url";

export function algaUrlsFromBase(raw: string): { httpBase: string } {
  const trimmed = (raw ?? "").trim();
  if (!trimmed) {
    return { httpBase: "" };
  }
  const withScheme = trimmed.includes("://") ? trimmed : `http://${trimmed}`;
  let u: URL;
  try {
    u = new URL(withScheme);
  } catch {
    return { httpBase: "" };
  }
  const scheme = (u.protocol || "http:").replace(/:$/, "").toLowerCase();
  const netloc = u.host;
  if (!netloc) {
    return { httpBase: "" };
  }

  if (scheme === "http" || scheme === "https") {
    const httpBase = `${scheme}://${netloc}`.replace(/\/$/, "");
    return { httpBase };
  }

  if (scheme === "ws" || scheme === "wss") {
    const httpScheme = scheme === "wss" ? "https" : "http";
    const httpBase = `${httpScheme}://${netloc}`.replace(/\/$/, "");
    return { httpBase };
  }

  return { httpBase: "" };
}
