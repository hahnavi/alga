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
import "./app.css";

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
