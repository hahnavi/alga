import mattermostIcon from "@/assets/mattermost-32x32.png";
import slackIcon from "@/assets/slack-35x34.png";

export function getProviderIconSrc(provider: string): string {
  return provider.toLowerCase() === "slack" ? slackIcon : mattermostIcon;
}
