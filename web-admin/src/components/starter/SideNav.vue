<script setup lang="ts">
import { computed } from 'vue';
import { $t } from '@/locales';
import { accessibleNav, NAV } from '@/components/starter/nav';
import { useAuthStore } from '@/store/modules/auth';

defineOptions({ name: 'StarterSideNav' });
defineProps<{ active: string; collapsed: boolean; expanded: string[] }>();
const emit = defineEmits<{ (e: 'change', value: string): void; (e: 'expand', value: string[]): void; (e: 'home'): void }>();

const authStore = useAuthStore();
const navItems = computed(() => accessibleNav(NAV, (resource, action) => authStore.hasPermission(resource, action)));
</script>

<template>
  <TAside :width="collapsed ? '64px' : '232px'" class="td-aside">
    <!-- Use a native button so the home affordance is reachable by keyboard. -->
    <button type="button" class="td-logo" :aria-label="$t('route.dashboard')" @click="emit('home')">
      <TIcon name="logo-github-filled" size="26px" />
      <span v-if="!collapsed" class="td-logo-text">GoDDI Console</span>
    </button>
    <TMenu
      :value="active"
      :expanded="expanded"
      :collapsed="collapsed"
      theme="light"
      @change="(v: string) => emit('change', v)"
      @expand="(v: string[]) => emit('expand', v)"
    >
      <template v-for="item in navItems" :key="item.key">
        <TSubmenu v-if="item.children" :value="item.key">
          <template #icon><TIcon :name="item.icon" /></template>
          <template #title>
            <button
              v-if="!collapsed"
              type="button"
              class="menu-key-target"
              :aria-expanded="expanded.includes(item.key)"
              @click.stop="emit('expand', expanded.includes(item.key) ? expanded.filter(key => key !== item.key) : [...expanded, item.key])"
            >
              {{ $t(item.labelKey as any) }}
            </button>
            <span v-else>{{ $t(item.labelKey as any) }}</span>
          </template>
          <TMenuItem
            v-for="child in item.children"
            :key="child.key"
            :value="child.key"
          >
            <template #icon>
              <button v-if="collapsed" type="button" class="menu-key-target menu-key-target--icon" :aria-label="$t(child.labelKey as any)" @click.stop="emit('change', child.key)">
                <TIcon :name="child.icon" />
              </button>
              <TIcon v-else :name="child.icon" />
            </template>
            <button v-if="!collapsed" type="button" class="menu-key-target" :aria-label="$t(child.labelKey as any)" @click.stop="emit('change', child.key)">
              <span>{{ $t(child.labelKey as any) }}</span>
            </button>
            <span v-else>{{ $t(child.labelKey as any) }}</span>
          </TMenuItem>
        </TSubmenu>
        <TMenuItem v-else :value="item.key">
          <template #icon>
            <button v-if="collapsed" type="button" class="menu-key-target menu-key-target--icon" :aria-label="$t(item.labelKey as any)" @click.stop="emit('change', item.key)">
              <TIcon :name="item.icon" />
            </button>
            <TIcon v-else :name="item.icon" />
          </template>
          <button v-if="!collapsed" type="button" class="menu-key-target" :aria-label="$t(item.labelKey as any)" @click.stop="emit('change', item.key)">
            <span>{{ $t(item.labelKey as any) }}</span>
          </button>
          <span v-else>{{ $t(item.labelKey as any) }}</span>
        </TMenuItem>
      </template>
    </TMenu>
  </TAside>
</template>

<style scoped>
/* The logo lives inside this component, so its styles belong here rather
   than in the parent shell (parent scoped CSS only reaches this root node). */
.td-logo {
  height: 56px;
  flex: 0 0 56px;
  display: flex;
  align-items: center;
  gap: 10px;
  width: 100%;
  padding: 0 18px;
  border: 0;
  border-bottom: 1px solid var(--shell-border);
  background: transparent;
  color: var(--shell-text);
  font: inherit;
  text-align: left;
  cursor: pointer;
  white-space: nowrap;
}
.td-logo:focus-visible,
.menu-key-target:focus-visible {
  outline: 2px solid var(--td-brand-color, #0052d9);
  outline-offset: -2px;
}
.td-logo-text {
  font-size: 16px;
  font-weight: 600;
  letter-spacing: 0.5px;
}
.menu-key-target--icon {
  width: 40px;
  height: 40px;
  justify-content: center;
  padding: 0;
}
.menu-key-target {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  min-height: 32px;
  padding: 0;
  border: 0;
  background: transparent;
  color: inherit;
  font: inherit;
  text-align: left;
  cursor: pointer;
}
</style>