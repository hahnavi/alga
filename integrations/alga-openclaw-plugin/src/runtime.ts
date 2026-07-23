import { createPluginRuntimeStore } from "openclaw/plugin-sdk/runtime-store";
import type { PluginRuntime } from "openclaw/plugin-sdk/runtime-store";

const { setRuntime: setAlgaRuntime, getRuntime: getAlgaRuntime } = createPluginRuntimeStore(
  "Alga channel runtime not initialized",
) as {
  setRuntime: (r: PluginRuntime) => void;
  getRuntime: () => PluginRuntime;
};

export { getAlgaRuntime, setAlgaRuntime };
