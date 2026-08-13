import { createApp } from 'vue'
import { createPinia } from 'pinia'
import ElementPlus from 'element-plus'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import 'element-plus/dist/index.css'
import App from './App.vue'
import router from './router'
import './styles/globals.scss'

// 不指定 locale 的话 Element Plus 一律出英文:分页是 "10/page"、"Go to",
// 确认弹窗的按钮是 "OK / Cancel" —— 界面其余部分全是中文。
createApp(App)
  .use(createPinia())
  .use(router)
  .use(ElementPlus, { locale: zhCn })
  .mount('#app')
