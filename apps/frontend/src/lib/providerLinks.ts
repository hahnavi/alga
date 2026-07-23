/** Provider (Slack/Mattermost) permalink + label helpers. */

/** Strip API suffix so permalinks use the team site root. */
function mattermostWebBaseFromConfig(url: string): string {
  let u = url.trim().replace(/\/$/, "");
  if (u.endsWith("/api/v4")) {
    u = u.slice(0, -"/api/v4".length);
  }
  return u.replace(/\/$/, "");
}

export function mattermostPostPermalink(baseUrl: string, postId: string, team?: string): string {
  const b = mattermostWebBaseFromConfig(baseUrl);
  const t = team?.trim();
  if (t) return `${b}/${t}/pl/${postId}`;
  return `${b}/pl/${postId}`;
}

/** Opens the Slack client to a channel message (works in browser with Slack signed in). */
export function slackMessageAppRedirectUrl(channelId: string, messageTs: string): string {
  const params = new URLSearchParams({
    channel: channelId.trim(),
    message: messageTs.trim(),
  });
  return `https://slack.com/app_redirect?${params.toString()}`;
}

/** Display name for Slack channels in the UI (prefix # when appropriate). */
export function formatSlackChannelLabel(channel: string): string {
  const c = channel.trim();
  if (!c) return c;
  if (c.startsWith("#")) return c;
  if (/^C[A-Z0-9]{8,}$/i.test(c)) return c;
  return `#${c}`;
}
