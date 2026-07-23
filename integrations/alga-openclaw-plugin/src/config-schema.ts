import { buildChannelConfigSchema } from "openclaw/plugin-sdk/channel-config-schema";
import { z } from "zod";

export const AlgaAccountConfigSchema = z
  .object({
    name: z.string().optional(),
    enabled: z.boolean().optional(),
    serverUrl: z.string().optional(),
    token: z.string().optional(),
    allowFrom: z.array(z.union([z.string(), z.number()])).optional(),
    defaultTo: z.string().optional(),
  })
  .strict();

export const AlgaChannelConfigSchema = AlgaAccountConfigSchema.extend({
  enabled: z.boolean().optional(),
  accounts: z.record(z.string(), AlgaAccountConfigSchema.partial()).optional(),
  defaultAccount: z.string().optional(),
}).strict();

export const algaChannelPluginConfigSchema = buildChannelConfigSchema(AlgaChannelConfigSchema);
