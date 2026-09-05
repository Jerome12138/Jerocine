<script setup lang="ts">
</script>

<template>
  <div
    class="min-h-screen flex-center bg-base text-primary p-[var(--gf-space-6)] relative overflow-hidden"
    :style="{ backgroundImage: `linear-gradient(135deg, #0b0b0f 0%, #1a1a2e 35%, #2d1b3d 70%, #1a1a2e 100%)` }"
  >
    <div
      class="absolute inset-0 pointer-events-none"
      :style="{ background: 'linear-gradient(135deg, rgba(11,11,15,0.6), rgba(106,19,187,0.45) 60%, rgba(11,11,15,0.85))' }"
    />
    <div
      class="relative w-full max-w-[440px] bg-glass rounded-[var(--gf-radius-2xl)] shadow-card-xl p-[var(--gf-space-8)] border border-default backdrop-blur-[18px]"
    >
      <slot />
    </div>
  </div>
</template>
