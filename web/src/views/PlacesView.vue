<script setup lang="ts">
import {t,locale} from '../i18n'

import { computed } from 'vue'
import { MapPin,ArrowUpRight,Compass } from 'lucide-vue-next'
import { useAlbum } from '../store'
import { thumbnail } from '../api'
const props=defineProps<{query:string;year:string;place:string}>(),emit=defineEmits<{place:[name:string]}>(),album=useAlbum()
const cities=computed(()=>{const result=new Map<string,{name:string;photos:number;memories:number;cover:string;province:string}>();for(const m of album.memories){if(props.year&&!m.date.startsWith(props.year)||props.place&&m.place!==props.place&&m.city!==props.place||props.query&&!`${m.title} ${m.place} ${m.city}`.toLowerCase().includes(props.query.toLowerCase()))continue;const name=m.city||m.place;if(!name)continue;const old=result.get(name);if(old){old.photos+=m.photos.length;old.memories++}else result.set(name,{name,photos:m.photos.length,memories:1,cover:m.cover_id||m.photos[0]?.id||'',province:m.province})}return [...result.values()]})
</script>
<template><div class="places-grid"><button v-for="city in cities" :key="city.name" class="place-card" @click="emit('place',city.name)"><img v-if="city.cover" :src="thumbnail(city.cover)" :alt="city.name" loading="lazy"><div><span class="place-overline"><MapPin :size="13"/>{{city.province||'A PLACE TO REMEMBER'}}</span><h2>{{city.name}}</h2><span>{{t('{0} 段回忆 · {1} 张照片', [city.memories,city.photos])}}<ArrowUpRight :size="19"/></span></div></button></div><div v-if="!cities.length" class="empty-state glass"><span class="empty-icon"><Compass :size="38"/></span><h2>{{t('下一站，会是哪里？')}}</h2><p>{{t('给回忆补充城市，同一城市的故事就会聚到一起。')}}</p></div></template>
