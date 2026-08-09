<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { api } from '@/api'
import type { TokenView } from '@/types'

const tokens = ref<TokenView[]>([])
const loading = ref(false)

const dialogOpen = ref(false)
const newName = ref('')
const issuing = ref(false)

const issued = ref<{ id: string; name: string; secret: string } | null>(null)
const revealOpen = ref(false)

async function load() {
  loading.value = true
  try {
    tokens.value = await api.listTokens()
  } catch (e: any) {
    ElMessage.error(e?.message || '加载失败')
  } finally {
    loading.value = false
  }
}

async function issue() {
  if (!newName.value.trim()) {
    ElMessage.warning('请填写用途名称')
    return
  }
  issuing.value = true
  try {
    const r = await api.createToken(newName.value.trim())
    issued.value = { id: r.id, name: r.name, secret: (r as any).secret }
    dialogOpen.value = false
    newName.value = ''
    revealOpen.value = true
    await load()
  } catch (e: any) {
    ElMessage.error(e?.message || '创建失败')
  } finally {
    issuing.value = false
  }
}

async function remove(t: TokenView) {
  if (t.role === 'admin') {
    ElMessage.warning('admin token 不能删除')
    return
  }
  try {
    await ElMessageBox.confirm(`确定删除 token "${t.name}" (${t.id})?`, '删除 token', {
      type: 'warning',
    })
  } catch { return }
  try {
    await api.deleteToken(t.id)
    ElMessage.success('已删除')
    await load()
  } catch (e: any) {
    ElMessage.error(e?.message || '删除失败')
  }
}

async function copySecret() {
  if (!issued.value) return
  try {
    await navigator.clipboard.writeText(issued.value.secret)
    ElMessage.success('已复制,请立即保管')
  } catch {}
}

onMounted(load)
</script>

<template>
  <section class="page">
    <div class="masthead">
      <div class="eyebrow">令牌 · TOKENS</div>
      <div class="row">
        <h1>API token</h1>
        <el-button type="primary" @click="dialogOpen = true">+ 新建 token</el-button>
      </div>
      <div class="sub">secret 只在创建时显示一次,请交给使用方后自行保管。</div>
    </div>

    <el-table :data="tokens" v-loading="loading" stripe empty-text="尚无 token">
      <el-table-column label="ID" prop="id" width="130">
        <template #default="{ row }">
          <span class="mono meta">{{ row.id }}</span>
        </template>
      </el-table-column>
      <el-table-column label="用途 / 名称" prop="name" min-width="160" />
      <el-table-column label="身份" prop="role" width="90">
        <template #default="{ row }">
          <span class="chip" :class="row.role">{{ row.role }}</span>
        </template>
      </el-table-column>
      <el-table-column label="创建别名数" prop="alias_count" width="120" align="right">
        <template #default="{ row }">
          <span class="count">{{ row.alias_count }}</span>
        </template>
      </el-table-column>
      <el-table-column label="上次使用" prop="last_used_at" min-width="180">
        <template #default="{ row }">
          <span class="mono" v-if="row.last_used_at">{{ new Date(row.last_used_at).toLocaleString() }}</span>
          <span class="dim" v-else>—</span>
        </template>
      </el-table-column>
      <el-table-column label="创建于" prop="created_at" min-width="180">
        <template #default="{ row }">
          <span class="mono">{{ new Date(row.created_at).toLocaleDateString() }}</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="120" align="right">
        <template #default="{ row }">
          <div class="acts">
            <el-button
              v-if="row.role !== 'admin'"
              link type="danger" size="small"
              @click="remove(row)"
            >删除</el-button>
            <span v-else class="dim">—</span>
          </div>
        </template>
      </el-table-column>
    </el-table>

    <!-- 创建对话框 -->
    <el-dialog v-model="dialogOpen" title="新建 token" width="440px">
      <el-form label-position="top">
        <el-form-item label="用途名称">
          <el-input v-model="newName" placeholder="例如:某业务方 · GitHub 注册脚本" @keyup.enter="issue" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button plain @click="dialogOpen = false">取消</el-button>
        <el-button type="primary" :loading="issuing" @click="issue">创建</el-button>
      </template>
    </el-dialog>

    <!-- Secret 只显示一次 -->
    <el-dialog v-model="revealOpen" width="520px" :show-close="false" :close-on-click-modal="false">
      <template #header>
        <div class="reveal-head">
          <span class="once-mark">仅此一次</span>
          <h2>token 已创建</h2>
        </div>
      </template>

      <div v-if="issued" class="reveal">
        <div class="row">
          <span class="k">id</span><span class="v mono">{{ issued.id }}</span>
        </div>
        <div class="row">
          <span class="k">name</span><span class="v">{{ issued.name }}</span>
        </div>
        <div class="secret">{{ issued.secret }}</div>
        <div class="warn">仅显示一次 · 请立即复制并交给使用方保管</div>
      </div>

      <template #footer>
        <el-button plain @click="revealOpen = false">我已保存</el-button>
        <el-button type="primary" @click="copySecret">复制 secret</el-button>
      </template>
    </el-dialog>
  </section>
</template>

<style lang="scss" scoped>
.page { max-width: 1080px; }
.masthead h1 {
  font-family: var(--f-display); font-weight: 700;
  font-size: 40px; letter-spacing: -0.02em; margin: 6px 0;
}
.masthead .row { display: flex; align-items: center; justify-content: space-between; }
.masthead .sub { color: var(--dim); font-size: 13px; }

.meta { color: var(--dim); font-size: 11px; }
.dim { color: var(--dim); }
.count {
  font-family: var(--f-display); font-weight: 700; font-size: 20px;
  font-variant-numeric: tabular-nums;
}

.acts {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  :deep(.el-button.is-link) {
    min-height: var(--hit);
    padding: 0 8px;
    margin-left: 0;
  }
}
.chip {
  font-family: var(--f-mono); font-size: 10px; letter-spacing: 0.14em;
  padding: 2px 8px; border: 1px solid var(--rule); text-transform: uppercase;
  &.admin { border-color: var(--accent); color: var(--accent); }
  &.user  { border-color: var(--primary); color: var(--primary); }
}

.reveal-head { display: flex; align-items: baseline; gap: 12px; }
.once-mark {
  border: 2px solid var(--accent); color: var(--accent);
  padding: 3px 8px; font-size: 10px; letter-spacing: 0.24em;
}
.reveal-head h2 {
  font-family: var(--f-display); font-size: 20px; margin: 0; letter-spacing: -0.01em;
}
.reveal .row {
  display: flex; gap: 20px; padding: 8px 0;
  border-bottom: 1px dashed var(--rule);
  .k { width: 60px; color: var(--dim); font-size: 11px; letter-spacing: 0.16em; text-transform: uppercase; }
  .v { font-size: 13px; }
}
.secret {
  margin: 20px 0 8px;
  font-family: var(--f-mono);
  font-size: 15px;
  padding: 16px;
  background: var(--bg);
  border: 1px dashed var(--ink);
  word-break: break-all;
  user-select: all;
}
.warn { color: var(--accent); font-size: 12px; letter-spacing: 0.08em; }
</style>
