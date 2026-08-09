<script setup lang="ts">
import { ref } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { useAuthStore } from '@/stores/auth'
import { ElMessage } from 'element-plus'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()

const token = ref('')
const loading = ref(false)

async function submit() {
  if (!token.value.trim()) {
    ElMessage.warning('请粘贴 token')
    return
  }
  loading.value = true
  try {
    await auth.login(token.value.trim())
    const dest = (route.query.r as string) || (auth.isAdmin ? '/overview' : '/aliases')
    router.push(dest)
  } catch (e: any) {
    ElMessage.error(e?.message || 'token 无效')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="page">
    <div class="stamp-frame">
      <div class="postmark">POST · REGISTRY</div>
      <div class="corner">Nº HME · 001</div>

      <div class="title">
        <div class="eyebrow">iCLOUD · HIDE MY EMAIL</div>
        <h1>档案登录</h1>
        <p class="sub">粘贴 admin 或子 token,凭它进入档案库。</p>
      </div>

      <el-input
        v-model="token"
        type="password"
        show-password
        placeholder="Bearer token / X-API-Key"
        @keyup.enter="submit"
        size="large"
      />
      <el-button type="primary" size="large" class="submit" :loading="loading" @click="submit">
        进入档案
      </el-button>

      <hr class="rule-dashed" />
      <div class="hint">
        没有 token 时,联系 admin 通过 <span class="mono">POST /api/tokens</span> 签发一枚。
      </div>
    </div>
  </div>
</template>

<style lang="scss" scoped>
.page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background:
    radial-gradient(circle at 20% 0%, rgba(27, 75, 143, 0.05) 0%, transparent 40%),
    radial-gradient(circle at 100% 100%, rgba(200, 37, 37, 0.05) 0%, transparent 40%),
    var(--bg);
}
.stamp-frame {
  position: relative;
  width: 480px;
  max-width: 100%;
  background: var(--paper);
  border: 1px solid var(--ink);
  padding: 40px 40px 32px;
  /* 邮票齿边 SVG mask */
  &::before, &::after {
    content: '';
    position: absolute;
    left: -4px; right: -4px;
    height: 4px;
    background-image: radial-gradient(circle, var(--bg) 3px, transparent 3.5px);
    background-size: 12px 12px;
    background-position: 0 -4px;
  }
  &::before { top: -4px; }
  &::after { bottom: -4px; background-position: 0 0; }
}
.postmark {
  position: absolute;
  top: 12px; right: -16px;
  transform: rotate(8deg);
  border: 2px solid var(--stamp);
  color: var(--stamp);
  padding: 6px 12px;
  font-family: var(--f-body);
  font-size: 10px;
  letter-spacing: 0.24em;
  background: var(--paper);
}
.corner {
  position: absolute;
  top: 14px; left: 20px;
  font-family: var(--f-mono);
  font-size: 10px;
  letter-spacing: 0.14em;
  color: var(--dim);
}
.title { margin: 32px 0 24px; }
.title h1 {
  font-family: var(--f-display);
  font-weight: 700;
  font-size: 40px;
  margin: 6px 0 8px;
  letter-spacing: -0.02em;
}
.sub { color: var(--dim); font-size: 14px; margin: 0; }

.submit { width: 100%; margin-top: 14px; }
.hint { font-size: 12px; color: var(--dim); }
</style>
