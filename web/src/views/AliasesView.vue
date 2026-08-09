<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { api } from '@/api'
import type { Account, Alias, CreateResult } from '@/types'
import StampReveal from '@/components/StampReveal.vue'
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

const reveal = ref<CreateResult | null>(null)
const revealOpen = ref(false)

const emptyHint = computed(() =>
  accountId.value ? '还没有邮票 — 点右上"贴一枚邮票"开始' : '选择一个 iCloud 账号',
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
    reveal.value = await api.createAlias(accountId.value, newLabel.value.trim())
    revealOpen.value = true
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
    ElMessage.success(a.active ? '已停用' : '已激活')
    await loadAliases()
  } catch (e: any) {
    ElMessage.error(e?.message || '操作失败')
  }
}

async function remove(a: Alias) {
  try {
    await ElMessageBox.confirm(`确定销毁 "${a.email}" ? 不可恢复`, '销毁确认', { type: 'warning' })
  } catch { return }
  try {
    await api.deleteAlias(a.anonymousId, accountId.value)
    ElMessage.success('已销毁')
    await loadAliases()
  } catch (e: any) {
    ElMessage.error(e?.message || '销毁失败')
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
          <div class="eyebrow">邮票 · ALIASES</div>
          <h1>你贴过的邮票</h1>
        </div>
        <el-button type="primary" size="large" @click="dialogOpen = true">+ 贴一枚邮票</el-button>
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
          <span class="email mono" @click="copy(row.email)">{{ row.email }}</span>
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
            {{ row.active ? 'active' : 'silent' }}
          </span>
        </template>
      </el-table-column>
      <el-table-column label="ID" prop="anonymousId" width="180">
        <template #default="{ row }">
          <span class="mono meta">{{ row.anonymousId }}</span>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="240" align="right">
        <template #default="{ row }">
          <el-button link type="primary" size="small" @click="openInbox(row)">收信</el-button>
          <el-button link size="small" @click="toggle(row)">{{ row.active ? '停用' : '激活' }}</el-button>
          <el-button link type="danger" size="small" @click="remove(row)">销毁</el-button>
        </template>
      </el-table-column>
    </el-table>

    <!-- 创建对话框 -->
    <el-dialog v-model="dialogOpen" title="贴一枚邮票" width="440px">
      <el-form label-position="top">
        <el-form-item label="标签(用途备注)">
          <el-input v-model="newLabel" placeholder="例如:GitHub 注册" @keyup.enter="create" />
        </el-form-item>
        <div class="hint">
          <span class="eyebrow">TIP</span>
          <span>池非空时会秒回一枚现成邮票;池空时会当场向 iCloud 申请。</span>
        </div>
      </el-form>
      <template #footer>
        <el-button plain @click="dialogOpen = false">取消</el-button>
        <el-button type="primary" :loading="creating" @click="create">贴上</el-button>
      </template>
    </el-dialog>

    <StampReveal :result="reveal" :open="revealOpen" @close="revealOpen = false" />
  </section>
</template>

<style lang="scss" scoped>
.page { max-width: 1080px; }

.masthead .row { display: flex; align-items: flex-end; justify-content: space-between; }
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
.count-line { color: var(--dim); font-size: 12px; margin-left: auto; }

.email {
  cursor: pointer;
  &:hover { color: var(--primary); }
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
