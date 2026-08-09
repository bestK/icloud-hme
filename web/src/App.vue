<script setup lang="ts">
import { useRoute } from 'vue-router'
import AppHeader from '@/components/AppHeader.vue'
import AppSidebar from '@/components/AppSidebar.vue'
import { useAuthStore } from '@/stores/auth'
import { computed } from 'vue'

const route = useRoute()
const auth = useAuthStore()

const chromeless = computed(() => route.meta.public === true || !auth.isLoggedIn)
</script>

<template>
  <div v-if="chromeless" class="chromeless">
    <router-view />
  </div>
  <div v-else class="shell">
    <AppHeader />
    <div class="body">
      <AppSidebar />
      <main class="main">
        <router-view />
      </main>
    </div>
  </div>
</template>

<style lang="scss" scoped>
.chromeless { min-height: 100vh; background: var(--bg); }

.shell {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  background: var(--bg);
}

.body {
  flex: 1;
  display: grid;
  grid-template-columns: 220px 1fr;
  min-height: 0;
}

.main {
  padding: 32px 40px 64px;
  overflow-x: auto;
}

@media (max-width: 720px) {
  .body { grid-template-columns: 1fr; }
  .main { padding: 20px 16px 40px; }
}
</style>
