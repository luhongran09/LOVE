<script setup lang="ts">
import {t,locale} from '../i18n'

import { computed,ref } from 'vue'
import { Maximize2,Minimize2,MapPin } from 'lucide-vue-next'
import { useAlbum } from '../store'
import { type Memory } from '../api'
import MemoryMap from '../components/MemoryMap.vue'
const props=defineProps<{query:string;year:string;place:string}>(),emit=defineEmits<{open:[memory:Memory]}>(),album=useAlbum(),full=ref(false)
const memories=computed(()=>album.memories.filter(m=>(!props.year||m.date.startsWith(props.year))&&(!props.place||m.city===props.place||m.place===props.place)&&(!props.query||`${m.title} ${m.place} ${m.city} ${m.description}`.toLowerCase().includes(props.query.toLowerCase()))))
const photos=computed(()=>memories.value.flatMap(m=>m.photos.map(p=>({...p,mode:m.mode,memory_id:m.id,memory_title:m.title,latitude:p.latitude??m.latitude,longitude:p.longitude??m.longitude,place:m.city||m.place||p.place}))))
</script>
<template><section class="map-panel glass" :class="{'map-fullscreen':full}"><div class="map-panel-header"><span><MapPin :size="16"/>{{t('我们的回忆地图')}}</span><div><button class="icon" :aria-label="full?t('退出地图全屏'):t('地图全屏')" @click="full=!full"><Minimize2 v-if="full" :size="16"/><Maximize2 v-else :size="16"/></button></div></div><MemoryMap :photos="photos" @open="photo=>{const memory=memories.find(m=>m.photos.some(p=>p.id===photo.id));if(memory)emit('open',memory)}"/><div class="map-caption">{{t('{0} 段回忆 · 点击分类图钉查看回忆，点击数字展开聚合。', [memories.length])}}</div></section></template>
