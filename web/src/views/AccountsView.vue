<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { api } from '@/api'
import type { Account } from '@/types'

const accounts = ref<Account[]>([])
const loading = ref(false)

// 新增
const addOpen = ref(false)
const addForm = ref({ name: '', cookies: '', host: 'icloud.com', proxy: '' })
const addLoading = ref(false)

// 更新 Cookies
const ckOpen = ref(false)
const ckAcc = ref<Account | null>(null)
const ckRaw = ref('')
const ckLoading = ref(false)

// 密码登录
const loginOpen = ref(false)
const loginAcc = ref<Account | null>(null)
const loginForm = ref({ password: '', otp: '' })
const loginLoading = ref(false)

// App Password
const appOpen = ref(false)
const appAcc = ref<Account | null>(null)
const appForm = ref({ icloud_email: '', app_password: '' })
const appLoading = ref(false)

async function load() {
  loading.value = true
  try {
    accounts.value = await api.listAccounts()
  } catch (e: any) {
    ElMessage.error(e?.message || '加载失败')
  } finally {
    loading.value = false
  }
}

async function submitAdd() {
  if (!addForm.value.name.trim()) return ElMessage.warning('填账号名称')
  addLoading.value = true
  try {
    await api.addAccount({
      name: addForm.value.name.trim(),
      cookies: addForm.value.cookies.trim() || undefined,
      host: addForm.value.host.trim() || undefined,
      proxy: addForm.value.proxy.trim() || undefined,
    })
    ElMessage.success('已添加')
    addOpen.value = false
    addForm.value = { name: '', cookies: '', host: 'icloud.com', proxy: '' }
    await load()
  } catch (e: any) {
    ElMessage.error(e?.message || '添加失败')
  } finally {
    addLoading.value = false
  }
}

function openCookies(a: Account) {
  ckAcc.value = a
  ckRaw.value = ''
  ckOpen.value = true
}

function parseCookies(raw: string): Record<string, string> | null {
  raw = raw.trim()
  if (!raw) return null
  if (raw.startsWith('{')) {
    try {
      const obj = JSON.parse(raw)
      if (obj && typeof obj === 'object') return obj
    } catch { /* fallthrough */ }
  }
  // Header string: name=value; name=value
  const out: Record<string, string> = {}
  for (const part of raw.split(';')) {
    const idx = part.indexOf('=')
    if (idx <= 0) continue
    const k = part.slice(0, idx).trim()
    const v = part.slice(idx + 1).trim().replace(/^"|"$/g, '')
    if (k) out[k] = v
  }
  return Object.keys(out).length ? out : null
}

async function submitCookies() {
  if (!ckAcc.value) return
  const parsed = parseCookies(ckRaw.value)
  if (!parsed) return ElMessage.warning('无法解析,请粘 JSON 或 Header String')
  ckLoading.value = true
  try {
    const r = await api.updateCookies(ckAcc.value.id, parsed)
    ElMessage.success(`已更新,共 ${r.cookies_count} 条`)
    ckOpen.value = false
    await load()
  } catch (e: any) {
    ElMessage.error(e?.message || '更新失败')
  } finally {
    ckLoading.value = false
  }
}

// 正在重新校验的账号 id,用来只给那一行的按钮转圈
const revalidatingId = ref('')

async function revalidate(a: Account) {
  revalidatingId.value = a.id
  try {
    const r = await api.revalidateAccount(a.id)
    ElMessage.success(`已核对:共 ${r.alias_total} 个别名,启用 ${r.alias_active} 个`)
    await load()
  } catch (e: any) {
    ElMessage.error(e?.message || '校验失败')
  } finally {
    revalidatingId.value = ''
  }
}

function openLogin(a: Account) {
  loginAcc.value = a
  loginForm.value = { password: '', otp: '' }
  loginOpen.value = true
}

async function submitLogin() {
  if (!loginAcc.value) return
  if (!loginForm.value.password) return ElMessage.warning('填 Apple ID 密码')
  loginLoading.value = true
  try {
    await api.loginAccount(loginAcc.value.id, loginForm.value.password, loginForm.value.otp || undefined)
    ElMessage.success('已获取 Cookie 并保存')
    loginOpen.value = false
    await load()
  } catch (e: any) {
    const msg = e?.message || '登录失败'
    if (msg.includes('otp') || msg.includes('2FA') || msg.includes('验证码')) {
      ElMessage.info('需要 2FA 验证码,填入后再次点登录')
    } else {
      ElMessage.error(msg)
    }
  } finally {
    loginLoading.value = false
  }
}

function openApp(a: Account) {
  appAcc.value = a
  appForm.value = { icloud_email: a.icloud_email || '', app_password: '' }
  appOpen.value = true
}

async function submitApp() {
  if (!appAcc.value) return
  if (!appForm.value.icloud_email || !appForm.value.app_password) {
    return ElMessage.warning('两项都填')
  }
  appLoading.value = true
  try {
    await api.setAppPassword(appAcc.value.id, appForm.value.icloud_email, appForm.value.app_password)
    ElMessage.success('App Password 已保存并连通 IMAP')
    appOpen.value = false
    await load()
  } catch (e: any) {
    ElMessage.error(e?.message || '设置失败')
  } finally {
    appLoading.value = false
  }
}

async function remove(a: Account) {
  try {
    await ElMessageBox.confirm(`确定删除账号 "${a.name}" (${a.id}) ?`, '删除账号', { type: 'warning' })
  } catch { return }
  try {
    await api.removeAccount(a.id)
    ElMessage.success('已删除')
    await load()
  } catch (e: any) {
    ElMessage.error(e?.message || '删除失败')
  }
}

async function copyId(id: string) {
  try { await navigator.clipboard.writeText(id); ElMessage.success('已复制') } catch {}
}

onMounted(load)
</script>

<template>
  <section class="page">
    <div class="masthead">
      <div class="row">
        <div>
          <div class="eyebrow">账号 · ACCOUNTS</div>
          <h1>iCloud 账号</h1>
        </div>
        <el-button type="primary" @click="addOpen = true">+ 添加账号</el-button>
      </div>
      <div class="sub">Cookie 大约 24 小时轮换 · 失效后重新粘贴或用密码登录</div>
    </div>

    <el-table :data="accounts" v-loading="loading" empty-text="还没有 iCloud 账号">
      <el-table-column label="名称" prop="name" min-width="140">
        <template #default="{ row }">
          <div class="row-name">{{ row.name }}</div>
          <button
            class="row-id mono copyable"
            type="button"
            :title="`复制 ${row.id}`"
            @click="copyId(row.id)"
          >{{ row.id }}</button>
        </template>
      </el-table-column>
      <el-table-column label="邮箱" min-width="220">
        <template #default="{ row }">
          <div class="mono">{{ row.icloud_email || row.real_email || '—' }}</div>
          <div class="dim small">host · {{ row.host }}</div>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="110">
        <template #default="{ row }">
          <span class="chip" :class="row.status">{{ row.status }}</span>
        </template>
      </el-table-column>
      <el-table-column label="别名" width="96" align="right">
        <template #default="{ row }">
          <!-- 没核对过时 alias_total 的 0 是"不知道",显示 0 会误导 -->
          <template v-if="row.alias_counted_at">
            <span class="num">{{ row.alias_total }}</span>
            <div class="small dim">启用 {{ row.alias_active }}</div>
          </template>
          <template v-else>
            <span class="num dim" title="尚未与 iCloud 核对过">—</span>
            <div class="small dim">未核对</div>
          </template>
        </template>
      </el-table-column>
      <el-table-column label="上次校验" min-width="170">
        <template #default="{ row }">
          <span v-if="row.last_validated" class="mono">
            {{ new Date(row.last_validated).toLocaleString() }}
          </span>
          <span v-else class="dim">—</span>
          <div v-if="row.last_error" class="warn small">{{ row.last_error }}</div>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="420" align="right">
        <template #default="{ row }">
          <div class="acts">
            <el-button
              link
              size="small"
              :loading="revalidatingId === row.id"
              @click="revalidate(row)"
            >重新校验</el-button>
            <el-button link type="primary" size="small" @click="openCookies(row)">更新 Cookie</el-button>
            <el-button link size="small" @click="openLogin(row)">密码登录</el-button>
            <el-button link size="small" @click="openApp(row)">App Password</el-button>
            <span class="sep" aria-hidden="true" />
            <el-button link type="danger" size="small" @click="remove(row)">删除</el-button>
          </div>
        </template>
      </el-table-column>
    </el-table>

    <!-- 新增账号 -->
    <el-dialog v-model="addOpen" title="添加 iCloud 账号" width="520px">
      <el-form label-position="top">
        <el-form-item label="名称">
          <el-input v-model="addForm.name" placeholder="主号 / 副号 / 项目名" />
        </el-form-item>
        <el-form-item label="host">
          <el-input v-model="addForm.host" placeholder="icloud.com 或 icloud.com.cn" />
        </el-form-item>
        <el-form-item label="Cookies(可选,支持 JSON 或 Header String)">
          <el-input
            v-model="addForm.cookies"
            type="textarea"
            :rows="4"
            placeholder='{"X-APPLE-WEBAUTH-TOKEN":"..."}  或  name=val; name=val'
          />
        </el-form-item>
        <el-form-item label="代理(可选)">
          <el-input v-model="addForm.proxy" placeholder="http://user:pass@host:port" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button plain @click="addOpen = false">取消</el-button>
        <el-button type="primary" :loading="addLoading" @click="submitAdd">添加</el-button>
      </template>
    </el-dialog>

    <!-- 更新 Cookies -->
    <el-dialog v-model="ckOpen" width="600px">
      <template #header>
        <div>
          <div class="eyebrow">更新 Cookie</div>
          <div class="dialog-title">{{ ckAcc?.name }} · {{ ckAcc?.id }}</div>
        </div>
      </template>
      <el-alert
        type="info" :closable="false"
        title="从浏览器 F12 → Application → Cookies 里复制"
        description="支持两种格式:JSON 对象 {\'key\':\'value\'} 或 Header 字符串 name=val; name=val。至少要有 X-APPLE-WEBAUTH-TOKEN / X-APPLE-WEBAUTH-USER / X-APPLE-WEBAUTH-HSA-TRUST / X-APPLE-DS-WEB-SESSION-TOKEN。"
        style="margin-bottom: 12px"
      />
      <el-input
        v-model="ckRaw" type="textarea" :rows="10"
        placeholder='{"X-APPLE-WEBAUTH-TOKEN":"..."}  或  X-APPLE-WEBAUTH-TOKEN=...; ...'
      />
      <template #footer>
        <el-button plain @click="ckOpen = false">取消</el-button>
        <el-button type="primary" :loading="ckLoading" @click="submitCookies">保存并校验</el-button>
      </template>
    </el-dialog>

    <!-- 密码登录 -->
    <el-dialog v-model="loginOpen" width="480px">
      <template #header>
        <div>
          <div class="eyebrow">密码登录</div>
          <div class="dialog-title">{{ loginAcc?.name }} · {{ loginAcc?.id }}</div>
        </div>
      </template>
      <el-alert
        type="warning" :closable="false"
        title="使用 iCloud 账号的常规密码(不是 App Password)"
        description="首次登录若启用 2FA 会拒绝,填入 6 位验证码再点一次即可。"
        style="margin-bottom: 12px"
      />
      <el-form label-position="top">
        <el-form-item label="Apple ID 密码">
          <el-input v-model="loginForm.password" type="password" show-password />
        </el-form-item>
        <el-form-item label="2FA 验证码(有则填)">
          <el-input v-model="loginForm.otp" placeholder="六位数字" maxlength="6" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button plain @click="loginOpen = false">取消</el-button>
        <el-button type="primary" :loading="loginLoading" @click="submitLogin">登录</el-button>
      </template>
    </el-dialog>

    <!-- App Password -->
    <el-dialog v-model="appOpen" width="480px">
      <template #header>
        <div>
          <div class="eyebrow">App Password</div>
          <div class="dialog-title">{{ appAcc?.name }} · {{ appAcc?.id }}</div>
        </div>
      </template>
      <el-alert
        type="info" :closable="false"
        title="用于 IMAP 收信"
        description="到 appleid.apple.com → 登录和安全 → App 专用密码 → 生成一条。格式 xxxx-xxxx-xxxx-xxxx。"
        style="margin-bottom: 12px"
      />
      <el-form label-position="top">
        <el-form-item label="iCloud 邮箱">
          <el-input v-model="appForm.icloud_email" placeholder="you@icloud.com" />
        </el-form-item>
        <el-form-item label="App Password">
          <el-input v-model="appForm.app_password" placeholder="xxxx-xxxx-xxxx-xxxx" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button plain @click="appOpen = false">取消</el-button>
        <el-button type="primary" :loading="appLoading" @click="submitApp">保存并测 IMAP</el-button>
      </template>
    </el-dialog>
  </section>
</template>

<style lang="scss" scoped>
.page { max-width: 1100px; }
.masthead .row { display: flex; align-items: flex-end; justify-content: space-between; }
.masthead h1 {
  font-family: var(--f-display); font-weight: 700;
  font-size: 40px; letter-spacing: -0.02em; margin: 6px 0;
}
.masthead .sub { color: var(--dim); font-size: 12px; margin-bottom: 16px; }

.row-name { font-family: var(--f-display); font-weight: 500; font-size: 15px; }
.row-id {
  display: block;
  color: var(--dim); font-size: 11px; margin-top: 2px;
}

/* 行内操作:每个到 40px,"删除"前加竖线,窄屏允许换行 */
.acts {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
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
.chip {
  font-family: var(--f-mono); font-size: 10px; letter-spacing: 0.14em;
  padding: 2px 8px; border: 1px solid var(--rule); text-transform: uppercase;
  &.active { border-color: var(--ok); color: var(--ok); }
  &.error { border-color: var(--accent); color: var(--accent); }
  &.pending { border-color: var(--dim); color: var(--dim); }
}
.num {
  font-family: var(--f-display); font-weight: 700; font-size: 20px;
  font-variant-numeric: tabular-nums;
}
/* 10px 带中文读不清,给到 11px */
.small { font-size: 11px; letter-spacing: 0.04em; font-variant-numeric: tabular-nums; }
.dim { color: var(--dim); }
.warn { color: var(--accent); }

.eyebrow { font-size: 10px; }
.dialog-title {
  font-family: var(--f-display); font-weight: 700;
  font-size: 18px; letter-spacing: -0.01em; margin-top: 2px;
}
</style>
