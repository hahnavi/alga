import hermesAgentIcon from "@/assets/hermes-agent-32x32.png";
import openclawAgentIcon from "@/assets/openclaw-32x32.png";
import algaAgentIcon from "@/assets/alga-agent-32x32.png";
import googleMeetIcon from "@/assets/google-meet-32x32.png";

export type AgentBrand = "alga" | "hermes" | "openclaw" | "google_meet" | "other";

export function getAgentAvatarSrc(agentType?: string): string {
  if (agentType === "openclaw") return openclawAgentIcon;
  if (agentType === "alga") return algaAgentIcon;
  return hermesAgentIcon;
}

/** Returns the brand-specific icon URL, or `null` for the `"other"` fallback (caller renders a generic icon). */
export function getAgentBrandIconSrc(brand: AgentBrand): string | null {
  if (brand === "alga") return algaAgentIcon;
  if (brand === "hermes") return hermesAgentIcon;
  if (brand === "openclaw") return openclawAgentIcon;
  if (brand === "google_meet") return googleMeetIcon;
  return null;
}
