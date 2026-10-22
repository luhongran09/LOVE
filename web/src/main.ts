import {startPwa} from './pwa'
import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { createRouter,createWebHashHistory } from 'vue-router'
import App from './App.vue'
import './style.css'
const router=createRouter({history:createWebHashHistory(),routes:[
 {path:'/',redirect:'/timeline'},
 {path:'/timeline',component:()=>import('./components/MemoryCollection.vue')},
 {path:'/places',component:()=>import('./views/PlacesView.vue')},
 {path:'/map',component:()=>import('./views/MapView.vue')},
 {path:'/settings',component:()=>import('./views/SettingsView.vue')},
 {path:'/globe',component:()=>import('./views/GlobeView.vue')},
 {path:'/tools',component:()=>import('./views/ToolsView.vue')},
 {path:'/:pathMatch(.*)*',redirect:'/timeline'},
]})
createApp(App).use(createPinia()).use(router).mount('#app')

if(import.meta.env.PROD)void startPwa()
