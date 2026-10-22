<script setup lang="ts">
import {t,locale} from '../i18n'

import { computed } from 'vue'
import { Images,Compass } from 'lucide-vue-next'
import { thumbnail,type Memory } from '../api'
const props=defineProps<{memory:Memory;compact?:boolean}>()
const ordered=computed(()=>{const cover=props.memory.photos.find(p=>p.id===props.memory.cover_id);return cover?[cover,...props.memory.photos.filter(p=>p.id!==cover.id)]:props.memory.photos})
const capacity=computed(()=>ordered.value.length<=1?1:ordered.value.length<=4?4:9)
const visible=computed(()=>ordered.value.slice(0,capacity.value))
</script>
<template><div class="memory-mosaic" :class="[`mosaic-${capacity}`,{compact}]">
 <div v-for="(photo,index) in visible" :key="photo.id" class="mosaic-cell"><img :src="thumbnail(photo.id)" :alt="photo.title||memory.title" loading="lazy"><span v-if="index===capacity-1&&ordered.length>capacity" class="mosaic-overflow">+{{ordered.length-capacity}}</span></div>
 <div v-for="n in Math.max(0,capacity-visible.length)" :key="`empty-${n}`" class="mosaic-cell mosaic-filler" aria-hidden="true"><Compass :size="23"/><span>{{t('和你，一起')}}</span></div>
 <span class="mosaic-count"><Images :size="12"/>{{t('{0} 张照片', [ordered.length])}}</span>
</div></template>
