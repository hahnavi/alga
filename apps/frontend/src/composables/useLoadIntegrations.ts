import { ref } from "vue";
import { api } from "@/lib/api";
import { getErrorMessage } from "@/lib/error";
import { useToast } from "@/lib/toast";

export function useLoadIntegrations() {
  const { push } = useToast();
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
    } catch (err) {
      // The integrations probe is optional but a hard failure should still
      // surface — a silent miss leaves the page rendering empty with no clue.
      push(getErrorMessage(err, "Failed to load integrations"), "error");
    }
  }

  return { mattermostBaseUrl, mattermostTeam, load };
}
