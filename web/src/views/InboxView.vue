<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { api } from '@/api'
import type { MailMessage } from '@/types'
import ListPager from '@/components/ListPager.vue'
import { usePagination } from '@/composables/usePagination'

const route = useRoute()
const router = useRouter()

const accountId = ref((route.query.account_id as string) || '')
const alias = ref((route.query.alias as string) || '')
const messages = ref<MailMessage[]>([])
const loading = ref(false)
const method = ref<'imap' | 'web_api' | ''>('')

const mode = ref<'html' | 'raw'>('html')

// 邮件是整块正文,一屏放不下几封,每页给得比表格少
const { page, pageSize, total, paged: pagedMessages, reset: resetPage } = usePagination(messages, 10)

// IMAP 的 UID 只在单个邮箱内唯一,收件箱和垃圾箱合并之后会撞号,
// 得连邮箱名一起当键,否则展开一封会连带展开另一封。
const keyOf = (m: MailMessage) => `${m.folder ?? ''}:${m.id}`

// 一次能返回 20 封,全部展开就是 20 个 iframe。默认只摊开最新那封,
// 其余点开再渲染。
const opened = ref(new Set<string>())
const isOpen = (m: MailMessage) => opened.value.has(keyOf(m))
function toggle(m: MailMessage) {
  const k = keyOf(m)
  if (opened.value.has(k)) opened.value.delete(k)
  else opened.value.add(k)
}

function snippet(text: string) {
  const one = (text || '').replace(/\s+/g, ' ').trim()
  return one.length > 110 ? `${one.slice(0, 110)}…` : one
}

async function load() {
  if (!accountId.value || !alias.value) { messages.value = []; return }
  loading.value = true
  try {
    const r = await api.inbox(accountId.value, alias.value)
    messages.value = r.messages || []
    method.value = r.method
    resetPage()
    opened.value = new Set(messages.value.slice(0, 1).map(keyOf))
  } catch (e: any) {
    ElMessage.error(e?.message || '读取失败')
    messages.value = []
  } finally {
    loading.value = false
  }
}

// 邮件 HTML 是外部内容:直接塞进面板 DOM,它的 CSS 会污染整页,<script> 更是
// 拿到同一份会话。这里放进沙箱 iframe,再用 CSP 掐掉脚本和外链样式。
// 不给 allow-scripts,所以 allow-same-origin 只是为了量高度,构不成逃逸面。
const FRAME_CSP =
  "default-src 'none'; img-src data: https: http:; style-src 'unsafe-inline'; font-src data:"

function frameDoc(html: string) {
  return `<!doctype html><html><head><meta charset="utf-8">
<meta http-equiv="Content-Security-Policy" content="${FRAME_CSP}">
<base target="_blank">
<style>
html,body{margin:0;padding:0;background:#FAF8F2;color:#0E1A2C;overflow-wrap:anywhere;
  font:14px/1.6 -apple-system,"Segoe UI","PingFang SC",sans-serif;}
img{max-width:100%;height:auto;}
a{color:#1B4B8F;}
</style></head><body>${html}</body></html>`
}

const FRAME_MAX_H = 720

// iframe 不会跟着内容自适应高度,load 之后量一次内容再回填
function fitFrame(e: Event) {
  const el = e.target as HTMLIFrameElement
  const doc = el.contentDocument
  if (!doc) return
  const h = Math.max(doc.body?.scrollHeight ?? 0, doc.documentElement?.scrollHeight ?? 0)
  el.style.height = `${Math.min(Math.max(h + 8, 60), FRAME_MAX_H)}px`
}

// 点查询/回车:先把参数同步进 URL(方便刷新和分享),再确保真的去读一次。
// 参数没变时 router.replace 是一次重复导航,下面的 watch 不会触发 ——
// 只把它当触发器,按钮点下去就毫无反应。
function apply() {
  if (!accountId.value || !alias.value) {
    ElMessage.warning('填上 account_id 和别名地址')
    return
  }
  const unchanged =
    route.query.account_id === accountId.value && route.query.alias === alias.value
  router.replace({ query: { account_id: accountId.value, alias: alias.value } })
  if (unchanged) load()
}

// URL 变化:浏览器前进后退,或从别名页带着参数跳进来
watch(() => [route.query.account_id, route.query.alias], ([acc, al]) => {
  accountId.value = (acc as string) || ''
  alias.value = (al as string) || ''
  load()
})
onMounted(load)
</script>

<template>
  <section class="page">
    <div class="masthead">
      <div class="eyebrow">收件箱 · INBOX</div>
      <h1>别名收件箱</h1>
      <div class="filter">
        <el-input v-model="accountId" placeholder="account_id" style="width: 240px" @keyup.enter="apply" />
        <el-input v-model="alias" placeholder="alias@icloud.com" style="width: 320px" @keyup.enter="apply" />
        <el-button type="primary" :loading="loading" @click="apply">查询</el-button>
        <span v-if="method" class="method mono">via {{ method }}</span>
        <el-radio-group v-model="mode" class="mode">
          <el-radio-button value="html">HTML</el-radio-button>
          <el-radio-button value="raw">RAW</el-radio-button>
        </el-radio-group>
      </div>
      <div v-if="method === 'web_api' && mode === 'html'" class="caveat">
        这个账号走的是 Web API(Cookie),上游只给摘要,拿不到 HTML 正文 ——
        设一个 App Password 改走 IMAP 才有。
      </div>
    </div>

    <div v-if="loading" class="empty">正在读取邮件…</div>
    <div v-else-if="!alias" class="empty">
      <div class="status-mark">未选择别名</div>
      <p>填一个别名地址,查看它收到的邮件。</p>
    </div>
    <div v-else-if="!messages.length" class="empty">
      <div class="status-mark">暂无邮件</div>
      <p>{{ alias }} 目前没有邮件。</p>
    </div>
    <ul v-else class="messages">
      <li v-for="m in pagedMessages" :key="keyOf(m)" class="message">
        <button class="head" type="button" :aria-expanded="isOpen(m)" @click="toggle(m)">
          <div class="from">
            <span class="eyebrow">来自</span>
            <span class="mono">{{ m.from }}</span>
            <span v-if="m.folder && m.folder !== 'INBOX'" class="folder">{{ m.folder }}</span>
          </div>
          <div class="subject">{{ m.subject || '(无主题)' }}</div>
          <div v-if="!isOpen(m)" class="preview">{{ snippet(m.preview) }}</div>
          <div class="foot">
            <span class="mono">{{ m.date }}</span>
            <span class="toggle">{{ isOpen(m) ? '收起' : '展开' }}</span>
          </div>
        </button>

        <div v-if="isOpen(m)" class="body">
          <iframe
            v-if="mode === 'html' && m.html"
            class="frame"
            sandbox="allow-same-origin allow-popups allow-popups-to-escape-sandbox"
            referrerpolicy="no-referrer"
            :title="m.subject || '邮件正文'"
            :srcdoc="frameDoc(m.html)"
            @load="fitFrame"
          />
          <template v-else>
            <div v-if="mode === 'html'" class="note">这封邮件没有 HTML 正文,下面是纯文本</div>
            <pre class="raw">{{ m.preview || '(正文为空)' }}</pre>
          </template>
        </div>
      </li>
    </ul>

    <ListPager v-model:page="page" v-model:page-size="pageSize" :total="total" />
  </section>
</template>

<style lang="scss" scoped>
.page { max-width: 900px; }
.masthead h1 {
  font-family: var(--f-display); font-weight: 700; font-size: 40px;
  margin: 6px 0 14px; letter-spacing: -0.02em;
}
.filter {
  display: flex; gap: 12px; align-items: center;
  padding: 12px 0;
  border-top: 1px dashed var(--rule);
  border-bottom: 1px dashed var(--rule);
  margin-bottom: 20px;
}
.method { color: var(--dim); font-size: 11px; margin-left: 8px; letter-spacing: 0.14em; }
.mode { margin-left: auto; }
.caveat {
  color: var(--dim); font-size: 12px; line-height: 1.6;
  padding: 10px 0 0;
  margin-top: -8px;
  margin-bottom: 12px;
}

.empty {
  padding: 60px 20px;
  text-align: center;
  border: 1px dashed var(--rule);
  color: var(--dim);
  background: var(--paper);
}
.status-mark {
  display: inline-block;
  border: 2px solid var(--dim);
  padding: 4px 12px;
  font-size: 12px;
  /* 中文,字距从 0.24em 收到 0.08em */
  letter-spacing: 0.08em;
  margin-bottom: 12px;
}

.messages { list-style: none; margin: 0; padding: 0; }
.message {
  border-bottom: 1px dashed var(--rule);
  padding: 18px 0;
  &:first-child { border-top: 1px dashed var(--rule); }
}

/* 整个信头是展开开关,所以是 button:键盘能 focus,读屏能念出展开状态 */
.head {
  display: block;
  width: 100%;
  text-align: left;
  background: none;
  border: 0;
  padding: 0;
  cursor: pointer;
  color: inherit;
  font: inherit;
  &:hover .subject { color: var(--primary); }
  &:focus-visible { outline: 2px solid var(--primary); outline-offset: 4px; }
}

.from { display: flex; gap: 12px; align-items: baseline; color: var(--dim); font-size: 11px; }
.folder {
  font-family: var(--f-mono); font-size: 10px; letter-spacing: 0.14em;
  padding: 1px 6px; border: 1px solid var(--accent); color: var(--accent);
  text-transform: uppercase;
}
.subject {
  margin: 6px 0 4px;
  font-family: var(--f-display); font-weight: 600; font-size: 20px;
  color: var(--ink); letter-spacing: -0.01em;
  transition: color var(--dur-fast) var(--ease-out);
}
.preview { color: var(--dim); font-size: 13px; line-height: 1.6; }
.foot {
  display: flex; gap: 14px; align-items: baseline;
  margin-top: 8px; font-size: 11px; color: var(--dim); letter-spacing: 0.08em;
}
.toggle { color: var(--primary); letter-spacing: 0.08em; }

.body { margin-top: 14px; }
.frame {
  display: block;
  width: 100%;
  height: 220px; /* load 之后由 fitFrame 按内容改写 */
  border: 1px solid var(--rule);
  background: var(--paper);
}
.note {
  color: var(--dim); font-size: 11px; letter-spacing: 0.08em;
  margin-bottom: 8px;
}
.raw {
  margin: 0;
  padding: 16px;
  background: var(--paper);
  border: 1px solid var(--rule);
  font-family: var(--f-mono); font-size: 12px; line-height: 1.7;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  max-height: 720px;
  overflow: auto;
}
</style>
