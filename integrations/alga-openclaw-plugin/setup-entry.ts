import { defineSetupPluginEntry } from "openclaw/plugin-sdk/channel-core";
import { algaChannelPlugin } from "./src/channel.js";

export default defineSetupPluginEntry(algaChannelPlugin);
