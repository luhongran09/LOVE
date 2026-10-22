<script setup lang="ts">
import {t,locale} from '../i18n'

import { onBeforeUnmount,ref } from 'vue'
import { X,ImagePlus,UploadCloud,Trash2 } from 'lucide-vue-next'
import { useModal } from '../useModal'
import PlaceSearch,{type PlaceResult} from './PlaceSearch.vue'
import { api,modes,type Photo,type MemoryMode } from '../api'
const props=defineProps<{memoryId?:string;initialFiles?:File[]}>()
const chosenCoordinates=ref<{latitude:number;longitude:number}|null>(null)
const dialog=useModal(),emit=defineEmits<{close:[];uploaded:[photos:Photo[],memoryID:string]}>()
function location(result:PlaceResult){place.value=result.name.slice(0,200);province.value=result.province;city.value=result.city;chosenCoordinates.value={latitude:result.latitude,longitude:result.longitude}}
const files=ref<File[]>([]),previews=ref<string[]>([]),busy=ref(false),error=ref(''),title=ref(''),place=ref(''),date=ref(''),note=ref(''),mode=ref<MemoryMode>('travel'),province=ref(''),city=ref('')
function choose(list:FileList|File[]|null){if(!list)return;previews.value.forEach(URL.revokeObjectURL);files.value=Array.from(list);previews.value=files.value.map(file=>URL.createObjectURL(file));error.value=''}
function remove(index:number){const remaining=files.value.filter((_,i)=>i!==index);choose(remaining)}
function paste(event:ClipboardEvent){const images=Array.from(event.clipboardData?.files||[]).filter(f=>f.type.startsWith('image/'));if(images.length){event.preventDefault();choose([...files.value,...images])}}
if(props.initialFiles?.length)choose(props.initialFiles)
onBeforeUnmount(()=>previews.value.forEach(URL.revokeObjectURL))
async function upload(){
 if(!files.value.length)return
 if(files.value.length>20||files.value.some(f=>f.size>20*1024*1024)||files.value.reduce((n,f)=>n+f.size,0)>100*1024*1024){error.value=t('每批最多 20 张、100MB，单张不超过 20MB');return}
 busy.value=true;error.value=''
 try{const data=new FormData();files.value.forEach(f=>data.append('photos',f));for(const [key,value] of Object.entries({title:title.value,mode:mode.value,place:place.value,taken_at:date.value,note:note.value,province:province.value,city:city.value,memory_id:props.memoryId||''}))data.append(key,value)
 if(chosenCoordinates.value){data.append('latitude',String(chosenCoordinates.value.latitude));data.append('longitude',String(chosenCoordinates.value.longitude))}
 const result=await api<{photos:Photo[];memory_id:string}>('/photos/upload',{method:'POST',body:data});emit('uploaded',result.photos,result.memory_id);emit('close')
 }catch(e){error.value=(e as Error).message}finally{busy.value=false}
}
</script>
<template><div class="overlay" @click.self="!busy&&emit('close')"><form ref="dialog" tabindex="-1" class="modal upload-modal" role="dialog" aria-modal="true" aria-labelledby="upload-title" @submit.prevent="upload" @keydown.esc="!busy&&emit('close')" @paste="paste"><button type="button" class="icon close" :disabled="busy" :aria-label="t('关闭')" @click="emit('close')"><X/></button><div class="eyebrow">COLLECT OUR LITTLE MOMENTS</div><h2 id="upload-title">{{memoryId?t('给这段回忆，再添几个瞬间'):t('一个故事，不止一张照片')}}</h2><p class="muted">{{memoryId?t('新照片会加入当前回忆，原有照片和顺序会保留。'):t('这次选择的照片会放在同一段回忆里。')}}</p>
 <label class="dropzone" @dragover.prevent @drop.prevent="choose($event.dataTransfer?.files||null)"><UploadCloud :size="35"/><strong>{{files.length?t('已选择 {0} 张照片',[files.length]):t('点击选择、拖入照片，或粘贴图片')}}</strong><span>{{t('JPG / PNG / GIF / WebP · 单张 20MB · 每批最多 20 张')}}</span><input type="file" accept="image/jpeg,image/png,image/gif,image/webp" multiple @change="choose(($event.target as HTMLInputElement).files)"></label>
 <div v-if="files.length" class="upload-previews"><div v-for="(file,index) in files" :key="`${file.name}-${index}`"><img :src="previews[index]" :alt="file.name"><span>{{index===0?t('默认封面'):index+1}}</span><button type="button" class="icon" :disabled="busy" :aria-label="t('移除{0}',[file.name])" @click="remove(index)"><Trash2 :size="12"/></button></div></div>
 <template v-if="!memoryId"><label>{{t('回忆标题')}}<input v-model="title" maxlength="120" :placeholder="t('例如：第一次一起去看海')"></label><label>{{t('回忆类型')}}<div class="upload-modes"><button v-for="(item,key) in modes" :key="key" type="button" :class="{active:mode===key}" :aria-pressed="mode===key" @click="mode=key">{{item.label}}</button></div></label></template>
 <div class="two-col"><label>{{t('共同地点（选填）')}}<input v-model="place" maxlength="200" :placeholder="t('例如：大理 · 洱海')"></label><label>{{t('拍摄日期（选填）')}}<input v-model="date" type="date"></label></div><PlaceSearch v-if="!memoryId" @select="location"/><label>{{t('共同备注（选填）')}}<textarea v-model="note" rows="2" maxlength="5000" :placeholder="t('今天有什么值得记住的？')"></textarea></label><p class="muted small">{{t('未填写日期时读取照片原始日期。照片的 EXIF 与原图会保留，之后可设置回忆封面和照片顺序。')}}</p><p v-if="error" class="error" role="alert">{{error}}</p><button class="primary wide" :disabled="busy||!files.length"><ImagePlus :size="18"/>{{busy?t('正在保存，请稍候…'):memoryId?t('追加到这段回忆'):t('收藏这段回忆')}}</button></form></div></template>
