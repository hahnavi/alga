import { z } from "zod";

// Belt-and-suspenders: the authoritative jitless flag is pre-seeded in
// zod-jitless.ts (imported before this module), but confirm via the public
// API in case Zod's internal config diverges from the global object.
z.config({ jitless: true });
