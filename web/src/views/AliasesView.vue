<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { api } from '@/api'
import type { Account, Alias, CreateResult } from '@/types'
import AliasCreated from '@/components/AliasCreated.vue'
import { useRouter } from 'vue-router'
import { useAuthStore } from '@/stores/auth'

const router = useRouter()
const auth = useAuthStore()

const accounts = ref<Account[]>([])
const accountId = ref<string>('')
const aliases = ref<Alias[]>([])
const loading = ref(false)

const dialogOpen = ref(false)
const newLabel = ref('')
const creating = ref(false)

const created = ref<CreateResult | null>(null)
const createdOpen = ref(false)

const emptyHint = computed(() =>
  accountId.value ? '还没有别名 — 点右上"新建别名"开始' : '选择一个 iCloud 账号',
)

async function loadAccounts() {
  try {
    if (auth.isAdmin) {
      accounts.value = await api.listAccounts()
    } else {
      // user 没有 /accounts 权限,退而求其次:让用户输入 account_id
      accounts.value = []
    }
    if (accounts.value.length && !accountId.value) {
      accountId.value = accounts.value[0].id
    }
  } catch (e: any) {
    ElMessage.error(e?.message || '账号加载失败')
  }
}

async function loadAliases() {
  if (!accountId.value) { aliases.value = []; return }
  loading.value = true
  try {
    const r = await api.listAliases(accountId.value)
    aliases.value = r.aliases || []
  } catch (e: any) {
    ElMessage.error(e?.message || '别名加载失败')
  } finally {
    loading.value = false
  }
}

async function create() {
  if (!accountId.value) return ElMessage.warning('先选账号')
  creating.value = true
  try {
    created.value = await api.createAlias(accountId.value, newLabel.value.trim())
    createdOpen.value = true
    dialogOpen.value = false
    newLabel.value = ''
    await loadAliases()
  } catch (e: any) {
    ElMessage.error(e?.message || '创建失败')
  } finally {
    creating.value = false
  }
}

async function toggle(a: Alias) {
  try {
    if (a.active) {
      await api.deactivateAlias(a.anonymousId, accountId.value)
    } else {
      await api.reactivateAlias(a.anonymousId, accountId.value)
    }
    ElMessage.success(a.active ? '已停用' : '已启用')
    await loadAliases()
  } catch (e: any) {
    ElMessage.error(e?.message || '操作失败')
  }
}

async function remove(a: Alias) {
  try {
    await ElMessageBox.confirm(`确定删除 "${a.email}" ? 不可恢复`, '删除别名', { type: 'warning' })
  } catch { return }
  try {
    await api.deleteAlias(a.anonymousId, accountId.value)
    ElMessage.success('已删除')
    await loadAliases()
  } catch (e: any) {
    ElMessage.error(e?.message || '删除失败')
  }
}

async function copy(text: string) {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success('已复制')
  } catch {}
}

function openInbox(a: Alias) {
  router.push({ path: '/inbox', query: { account_id: accountId.value, alias: a.email } })
}

watch(accountId, loadAliases)
onMounted(async () => {
  await loadAccounts()
  await loadAliases()
})
</script>

<template>
  <section class="page">
    <div class="masthead">
      <div class="row">
        <div>
          <div class="eyebrow">别名 · ALIASES</div>
          <h1>别名地址</h1>
        </div>
        <div class="row-acts">
          <el-button plain size="large" :loading="loading" @click="loadAliases">刷新</el-button>
          <el-button type="primary" size="large" @click="dialogOpen = true">+ 新建别名</el-button>
        </div>
      </div>

      <div class="filter">
        <div v-if="auth.isAdmin && accounts.length">
          <span class="eyebrow">账号</span>
          <el-select v-model="accountId" style="width: 240px; margin-left: 12px;">
            <el-option v-for="a in accounts" :key="a.id" :label="`${a.name} · ${a.id}`" :value="a.id" />
          </el-select>
        </div>
        <div v-else>
          <span class="eyebrow">账号</span>
          <el-input v-model="accountId" placeholder="acc_xxxxxxxx" style="width: 240px; margin-left: 12px;" />
        </div>
        <span class="count-line mono">count · {{ aliases.length }}</span>
      </div>
    </div>

    <el-table :data="aliases" v-loading="loading" :empty-text="emptyHint" stripe>
      <el-table-column label="地址" prop="email" min-width="240">
        <template #default="{ row }">
          <button
            class="email mono copyable"
            type="button"
            :title="`复制 ${row.email}`"
            @click="copy(row.email)"
          >{{ row.email }}</button>
        </template>
      </el-table-column>
      <el-table-column label="标签" prop="label" min-width="140">
        <template #default="{ row }">
          <span v-if="row.label" class="tag">{{ row.label }}</span>
          <span v-else class="dim">—</span>
        </template>
      </el-table-column>
      <el-table-column label="状态" prop="active" width="100">
        <template #default="{ row }">
          <span class="chip" :class="row.active ? 'active' : 'inactive'">
            {{ row.active ? 'active' : 'inactive' }}
          </span>
        </template>
      </el-table-column>
      <el-table-column label="ID" prop="anonymousId" width="180">
        <template #default="{ row }">
          <span class="mono meta">{{ row.anonymousId }}</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="260" align="right">
        <template #default="{ row }">
          <div class="acts">
            <el-button link type="primary" size="small" @click="openInbox(row)">收件箱</el-button>
            <el-button link size="small" @click="toggle(row)">{{ row.active ? '停用' : '启用' }}</el-button>
            <span class="sep" aria-hidden="true" />
            <el-button link type="danger" size="small" @click="remove(row)">删除</el-button>
          </div>
        </template>
      </el-table-column>
    </el-table>

    <!-- 创建对话框 -->
    <el-dialog v-model="dialogOpen" title="新建别名" width="440px">
      <el-form label-position="top">
        <el-form-item label="标签(用途备注)">
          <el-input v-model="newLabel" placeholder="例如:GitHub 注册" @keyup.enter="create" />
        </el-form-item>
        <div class="hint">
          <span class="eyebrow">TIP</span>
          <span>池里有现成地址时立即返回;池空时当场向 iCloud 申请,会慢几秒。</span>
        </div>
      </el-form>
      <template #footer>
        <el-button plain @click="dialogOpen = false">取消</el-button>
        <el-button type="primary" :loading="creating" @click="create">创建</el-button>
      </template>
    </el-dialog>

    <AliasCreated :result="created" :open="createdOpen" @close="createdOpen = false" />
  </section>
</template>

<style lang="scss" scoped>
.page { max-width: 1080px; }

.masthead .row { display: flex; align-items: flex-end; justify-content: space-between; }
.row-acts { display: flex; gap: 10px; }
.masthead h1 {
  font-family: var(--f-display); font-weight: 700;
  font-size: 40px; letter-spacing: -0.02em; margin: 6px 0;
}
.filter {
  display: flex; align-items: center; gap: 24px;
  margin: 16px 0 20px;
  padding: 14px 0;
  border-top: 1px dashed var(--rule);
  border-bottom: 1px dashed var(--rule);
}
.count-line {
  color: var(--dim); font-size: 12px; margin-left: auto;
  font-variant-numeric: tabular-nums;
}

.email { word-break: break-all; }

/* 行内操作:给足 40px 点击区,并把"删除"用一条竖线隔开 */
.acts {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 4px;
  :deep(.el-button.is-link) {
    min-height: var(--hit);
    padding: 0 8px;
    margin-left: 0;
  }
}
.sep {
  width: 1px;
  height: 16px;
  margin: 0 4px;
  background: var(--rule);
  flex: none;
}
.tag {
  font-family: var(--f-mono); font-size: 11px;
  border: 1px solid var(--rule); padding: 2px 8px;
}
.meta { color: var(--dim); font-size: 11px; }
.dim { color: var(--dim); }
.chip {
  font-family: var(--f-mono); font-size: 10px; letter-spacing: 0.14em;
  padding: 2px 8px; border: 1px solid var(--rule); text-transform: uppercase;
  &.active { border-color: var(--ok); color: var(--ok); }
  &.inactive { border-color: var(--dim); color: var(--dim); }
}
.hint {
  display: flex; gap: 12px; align-items: baseline;
  color: var(--dim); font-size: 12px; margin-top: 4px;
}
</style>
