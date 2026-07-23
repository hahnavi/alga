import { ref } from "vue";
import { api } from "@/lib/api";

export function useLoadIntegrations() {
  const mattermostBaseUrl = ref("");
  const mattermostTeam = ref("");
  const loaded = ref(false);

  async function load() {
    if (loaded.value) return;
    try {
      const integ = await api.getIntegrations();
      if (integ.mattermost?.base_url) mattermostBaseUrl.value = integ.mattermost.base_url;
      else if (integ.mattermost?.url) mattermostBaseUrl.value = integ.mattermost.url;
      if (integ.mattermost?.team) mattermostTeam.value = integ.mattermost.team;
      loaded.value = true;
    } catch {
      /* optional */
    }
  }

  return { mattermostBaseUrl, mattermostTeam, load };
}
