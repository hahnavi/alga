<script setup lang="ts">
import { useRouteLoading } from "@/composables/useRouteLoading";
import router from "@/router";

const tracker = useRouteLoading(router);
</script>

<template>
  <div
    class="route-loading-bar"
    :class="{
      'route-loading-bar--active': tracker.loading.value,
      'route-loading-bar--completing': tracker.completing.value,
    }"
    role="progressbar"
    aria-hidden="true"
  >
    <div class="route-loading-bar__indicator" />
  </div>
</template>

<style scoped>
.route-loading-bar {
  position: fixed;
  top: 0;
  left: 0;
  right: 0;
  height: 2px;
  z-index: 60;
  pointer-events: none;
  overflow: hidden;
  background: transparent;
}

.route-loading-bar__indicator {
  display: block;
  height: 100%;
  width: 100%;
  background: var(--focus-ring);
  box-shadow: 0 0 6px var(--focus-ring);
  transform-origin: left center;
  transform: scaleX(0);
  will-change: transform, opacity;
  transition:
    transform 180ms ease-out,
    opacity 180ms ease-out;
}

.route-loading-bar--active .route-loading-bar__indicator {
  transform: scaleX(0.9);
  transition: transform 1.6s cubic-bezier(0.1, 0.05, 0.3, 1);
}

.route-loading-bar--completing .route-loading-bar__indicator {
  transform: scaleX(1);
  opacity: 0;
  transition:
    transform 180ms ease-out,
    opacity 180ms ease-out;
}

@media (prefers-reduced-motion: reduce) {
  .route-loading-bar__indicator,
  .route-loading-bar--active .route-loading-bar__indicator,
  .route-loading-bar--completing .route-loading-bar__indicator {
    transition: none;
  }
}
</style>
