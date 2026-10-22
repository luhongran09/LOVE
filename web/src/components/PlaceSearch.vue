<script setup lang="ts">
import {t,locale} from '../i18n'

import { ref } from 'vue'
import { Search,LocateFixed } from 'lucide-vue-next'
import { api } from '../api'
export interface PlaceResult{name:string;province:string;city:string;latitude:number;longitude:number}
const props=defineProps<{latitude?:number|null;longitude?:number|null}>(),emit=defineEmits<{select:[place:PlaceResult]}>()
const query=ref(''),results=ref<PlaceResult[]>([]),error=ref(''),busy=ref(false)
async function search(reverse=false){busy.value=true;error.value='';results.value=[];try{if(reverse){const result=await api<PlaceResult>(`/places/reverse?lat=${props.latitude}&lon=${props.longitude}`);emit('select',result)}else{const result=await api<{places:PlaceResult[]}>(`/places/search?q=${encodeURIComponent(query.value)}`);results.value=result.places;if(!result.places.length)error.value=t('没有找到地点，可改用更完整的名称，或手动填写。')}}catch(e){error.value=(e as Error).message}finally{busy.value=false}}
</script>
<template><section class="place-search"><div><input v-model="query" :aria-label="t('搜索地名')" :placeholder="t('搜索城市、景点或地址')" @keydown.enter.prevent="query.trim().length>=2&&search()"><button type="button" class="secondary" :disabled="busy||query.trim().length<2" @click="search()"><Search :size="14"/>{{busy?t('查询中…'):t('查找')}}</button></div><button v-if="latitude!=null&&longitude!=null" type="button" class="text-button" :disabled="busy" @click="search(true)"><LocateFixed :size="13"/>{{t('根据照片 GPS 识别城市和地址')}}</button><p v-if="error" class="small muted" role="status">{{error}}</p><div v-if="results.length" class="place-results"><button v-for="result in results" :key="`${result.latitude},${result.longitude}`" type="button" @click="emit('select',result);results=[]"><strong>{{result.city||result.province}}</strong><span>{{result.name}}</span></button></div></section></template>
