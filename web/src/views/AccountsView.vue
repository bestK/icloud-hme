<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { api } from '@/api'
import type { Account, LoginDone } from '@/types'

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

// 密码登录:分两步。验证码是 Apple 在密码通过之后才发的,不可能跟密码同时填。
const loginOpen = ref(false)
const loginAcc = ref<Account | null>(null)
const loginStep = ref<'password' | 'code'>('password')
const loginForm = ref({ password: '', code: '' })
const loginId = ref('')
const loginAppleId = ref('')
const loginLoading = ref(false)
// 待验证会话在服务端有 TTL,过期得从密码重来,所以把剩余时间摆出来
const loginLeft = ref(0)
let loginTimer: ReturnType<typeof setInterval> | undefined

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
  loginStep.value = 'password'
  loginForm.value = { password: '', code: '' }
  loginId.value = ''
  loginAppleId.value = ''
  stopCountdown()
  loginOpen.value = true
}

function closeLogin() {
  loginOpen.value = false
  loginForm.value = { password: '', code: '' }
  loginId.value = ''
  stopCountdown()
}

function startCountdown(seconds: number) {
  stopCountdown()
  loginLeft.value = seconds
  loginTimer = setInterval(() => {
    loginLeft.value -= 1
    if (loginLeft.value <= 0) stopCountdown()
  }, 1000)
}

function stopCountdown() {
  if (loginTimer) {
    clearInterval(loginTimer)
    loginTimer = undefined
  }
  loginLeft.value = 0
}

// 登录实际用的账号名:和后端 LoginStart 的取值顺序保持一致
const loginEmail = computed(
  () => loginAcc.value?.icloud_email || loginAcc.value?.real_email || '',
)

const loginLeftText = computed(() => {
  const s = Math.max(0, loginLeft.value)
  return `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`
})

// 第一步:提交密码。开了 2FA 的账号到这里 Apple 才会把验证码发出去。
async function submitPassword() {
  if (!loginAcc.value) return
  if (!loginForm.value.password) return ElMessage.warning('填 Apple ID 密码')
  loginLoading.value = true
  try {
    const r = await api.loginAccount(loginAcc.value.id, loginForm.value.password)
    if (r.status === 'needs_2fa') {
      loginId.value = r.login_id
      loginAppleId.value = r.apple_id
      loginStep.value = 'code'
      loginForm.value.password = '' // 后面这步不再需要密码,不留在内存里
      startCountdown(r.expires_in)
      ElMessage.info('验证码已发到受信任设备')
      return
    }
    await settleLogin(r)
  } catch (e: any) {
    ElMessage.error(e?.message || '登录失败')
  } finally {
    loginLoading.value = false
  }
}

// 第二步:提交验证码,复用第一步那个 Apple 会话
async function submitCode() {
  if (!loginAcc.value) return
  const code = loginForm.value.code.trim()
  if (!/^\d{6}$/.test(code)) return ElMessage.warning('验证码是 6 位数字')
  loginLoading.value = true
  try {
    await settleLogin(await api.verifyLogin(loginAcc.value.id, loginId.value, code))
  } catch (e: any) {
    ElMessage.error(e?.message || '验证失败')
    loginForm.value.code = ''
    // 410 = 服务端那份待验证会话没了,只能从密码重来
    if (e?.status === 410) {
      loginStep.value = 'password'
      loginId.value = ''
      stopCountdown()
    }
  } finally {
    loginLoading.value = false
  }
}

async function settleLogin(r: LoginDone) {
  if (r.validated) {
    ElMessage.success(`登录成功,已保存 ${r.cookies_count} 条 Cookie`)
  } else {
    // Cookie 拿到了但用不了,别报成功
    ElMessage.warning(r.warning || 'Cookie 已保存,但会话校验没通过')
  }
  closeLogin()
  await load()
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
onUnmounted(stopCountdown)
</script>

<template>
  <section class="page">
    <div class="masthead">
      <div class="row">
        <div>
          <div class="eyebrow">账号 · ACCOUNTS</div>
          <h1>iCloud 账号</h1>
        </div>
        <div class="row-acts">
          <el-button plain :loading="loading" @click="load">刷新</el-button>
          <el-button type="primary" @click="addOpen = true">+ 添加账号</el-button>
        </div>
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

    <!-- 密码登录:第 1 步密码,第 2 步验证码 -->
    <el-dialog v-model="loginOpen" width="480px" @closed="closeLogin">
      <template #header>
        <div>
          <div class="eyebrow">密码登录 · 第 {{ loginStep === 'password' ? 1 : 2 }} / 2 步</div>
          <div class="dialog-title">{{ loginAcc?.name }} · {{ loginAcc?.id }}</div>
        </div>
      </template>

      <template v-if="loginStep === 'password'">
        <!-- 密码是给哪个 Apple ID 的,必须让人看见:一个面板下挂多个账号时,
             对着错误的账号输密码只会换来一句"Apple ID 或密码不正确" -->
        <div class="acct-line" :class="{ missing: !loginEmail }">
          <span class="k">Apple ID</span>
          <span v-if="loginEmail" class="mono v">{{ loginEmail }}</span>
          <span v-else class="v warn">未设置 —— 先粘一次 Cookie 或设置 App Password 把邮箱补上</span>
        </div>
        <el-alert
          type="warning" :closable="false"
          title="用 Apple ID 的常规密码,不是 App Password"
          description="App Password 只能用于 IMAP 收信,走不通这里的登录流程。账号开了双重认证的话,提交密码之后 Apple 才会把验证码发到受信任设备,下一步再填。"
          style="margin-bottom: 12px"
        />
        <el-form label-position="top" @submit.prevent="submitPassword">
          <el-form-item label="Apple ID 密码">
            <el-input
              v-model="loginForm.password"
              type="password"
              show-password
              :disabled="!loginEmail"
              @keyup.enter="submitPassword"
            />
          </el-form-item>
        </el-form>
      </template>

      <template v-else>
        <div class="acct-line">
          <span class="k">Apple ID</span>
          <span class="mono v">{{ loginAppleId || loginEmail }}</span>
        </div>
        <el-alert
          type="info" :closable="false"
          title="验证码已发到该账号的受信任设备"
          description="填这台设备上弹出的 6 位数字。这一步复用刚才那次登录会话,不要回去重新提交密码 —— 那会让 Apple 重发一个新码,手上这个当场作废。"
          style="margin-bottom: 12px"
        />
        <el-form label-position="top" @submit.prevent="submitCode">
          <el-form-item label="验证码">
            <el-input
              v-model="loginForm.code"
              placeholder="六位数字"
              maxlength="6"
              inputmode="numeric"
              autofocus
              @keyup.enter="submitCode"
            />
          </el-form-item>
        </el-form>
        <div class="dim small">
          <template v-if="loginLeft > 0">本次会话剩余 {{ loginLeftText }},超时要从密码重来</template>
          <template v-else>会话可能已超时,提交后若报失效请从密码重来</template>
        </div>
      </template>

      <template #footer>
        <el-button plain @click="loginOpen = false">取消</el-button>
        <el-button
          v-if="loginStep === 'password'"
          type="primary" :loading="loginLoading" :disabled="!loginEmail"
          @click="submitPassword"
        >下一步</el-button>
        <el-button v-else type="primary" :loading="loginLoading" @click="submitCode">
          完成登录
        </el-button>
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
.row-acts { display: flex; gap: 10px; }
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

/* 弹窗里的"这条操作作用在谁身上" */
.acct-line {
  display: flex;
  align-items: baseline;
  gap: 10px;
  padding: 8px 10px;
  margin-bottom: 10px;
  border: 1px solid var(--rule);
  .k {
    flex: none;
    font-size: 10px; letter-spacing: 0.14em; text-transform: uppercase;
    color: var(--dim);
  }
  .v { font-size: 13px; word-break: break-all; }
  &.missing { border-color: var(--accent); }
}

.eyebrow { font-size: 10px; }
.dialog-title {
  font-family: var(--f-display); font-weight: 700;
  font-size: 18px; letter-spacing: -0.01em; margin-top: 2px;
}
</style>
