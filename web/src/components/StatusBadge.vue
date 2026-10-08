<script setup lang="ts">
import { statusLabels, type JobStatus } from "../types/job";
defineProps<{ status: JobStatus }>();
</script>

<template>
  <span class="status-badge" :class="`status-${status}`"
    ><span class="status-dot" />{{ statusLabels[status] }}</span
  >
</template>
