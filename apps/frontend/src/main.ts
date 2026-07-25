import { createApp } from "vue";
import { createPinia } from "pinia";
import {
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  ArcElement,
  BarElement,
  Title,
  Tooltip,
  Legend,
  Filler,
} from "chart.js";
import App from "./App.vue";
import router from "./router";
import { z } from "zod";
import "./app.css";

// Zod v4 JIT-compiles object parsers with `new Function`, probing eval
// support at first parse. The strict CSP (script-src 'self', no
// 'unsafe-eval') makes that probe throw: Zod falls back gracefully, but the
// browser still reports a CSP violation. jitless skips dynamic codegen
// entirely so production runs clean under the strict policy.
z.config({ jitless: true });

ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  ArcElement,
  BarElement,
  Title,
  Tooltip,
  Legend,
  Filler,
);

async function bootstrap() {
  const app = createApp(App).use(createPinia()).use(router);
  try {
    await router.isReady();
  } catch (err) {
    console.error("Router failed to initialize:", err);
  }
  app.mount("#app");
}

bootstrap().catch((err) => {
  console.error("Failed to bootstrap app:", err);
});
