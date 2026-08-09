<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { api } from '@/api'
import type { MailMessage } from '@/types'

const route = useRoute()
const router = useRouter()

const accountId = ref((route.query.account_id as string) || '')
const alias = ref((route.query.alias as string) || '')
const messages = ref<MailMessage[]>([])
const loading = ref(false)
const method = ref<'imap' | 'web_api' | ''>('')

async function load() {
  if (!accountId.value || !alias.value) { messages.value = []; return }
  loading.value = true
  try {
    const r = await api.inbox(accountId.value, alias.value)
    messages.value = r.messages || []
    method.value = r.method
  } catch (e: any) {
    ElMessage.error(e?.message || '读取失败')
    messages.value = []
  } finally {
    loading.value = false
  }
}

function apply() {
  router.replace({ query: { account_id: accountId.value, alias: alias.value } })
}
watch(() => route.query, () => {
  accountId.value = (route.query.account_id as string) || ''
  alias.value = (route.query.alias as string) || ''
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
        <el-input v-model="accountId" placeholder="account_id" style="width: 240px" />
        <el-input v-model="alias" placeholder="alias@icloud.com" style="width: 320px" @keyup.enter="apply" />
        <el-button type="primary" @click="apply">查询</el-button>
        <span v-if="method" class="method mono">via {{ method }}</span>
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
      <li v-for="m in messages" :key="m.id" class="message">
        <div class="from">
          <span class="eyebrow">来自</span>
          <span class="mono">{{ m.from }}</span>
        </div>
        <div class="subject">{{ m.subject || '(无主题)' }}</div>
        <div class="preview">{{ m.preview }}</div>
        <div class="foot">
          <span class="mono">{{ m.date }}</span>
        </div>
      </li>
    </ul>
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
.from { display: flex; gap: 12px; align-items: baseline; color: var(--dim); font-size: 11px; }
.subject {
  margin: 6px 0 4px;
  font-family: var(--f-display); font-weight: 600; font-size: 20px;
  color: var(--ink); letter-spacing: -0.01em;
}
.preview { color: var(--dim); font-size: 13px; line-height: 1.6; }
.foot {
  margin-top: 8px; font-size: 11px; color: var(--dim); letter-spacing: 0.08em;
}
</style>
