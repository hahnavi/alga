import type { UserInfo } from "@/lib/api";

export function resolveDisplayName(opts: {
  userId?: string;
  username?: string;
  users: UserInfo[];
  role?: string;
  agentName?: string;
  fallback?: string;
}): string {
  const { userId, username, users, role, agentName, fallback } = opts;

  if (role === "agent") return agentName || "Agent";

  if (userId) {
    const byID = users.find((u) => u.id === userId);
    if (byID?.full_name?.trim()) return byID.full_name.trim();
    if (byID?.email?.trim()) return byID.email.trim();
  }

  if (username?.trim()) {
    const byEmail = users.find((u) => u.email === username);
    if (byEmail?.full_name?.trim()) return byEmail.full_name.trim();
    return username.trim();
  }

  return fallback ?? "You";
}
