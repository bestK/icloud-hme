<script setup lang="ts">
import { useRouter } from 'vue-router'
const router = useRouter()
</script>

<template>
  <div class="page">
    <div class="card">
      <div class="code">404</div>
      <h1>页面不存在</h1>
      <p>这个地址没有对应的页面。</p>
      <button class="back" @click="router.push('/')">返回首页</button>
    </div>
  </div>
</template>

<style lang="scss" scoped>
.page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--bg);
}
.card {
  border: 1px dashed var(--ink);
  padding: 40px 60px;
  background: var(--paper);
  text-align: center;
  position: relative;
}
.code {
  border: 2px solid var(--accent);
  color: var(--accent);
  display: inline-block;
  padding: 4px 12px;
  font-family: var(--f-mono);
  font-size: 12px;
  letter-spacing: 0.14em;
  margin-bottom: 16px;
}
h1 { font-family: var(--f-display); font-size: 42px; margin: 0 0 8px; }
p { color: var(--dim); }
.back {
  display: inline-flex; align-items: center; justify-content: center;
  min-height: var(--hit);
  margin-top: 16px;
  background: var(--ink);
  color: var(--paper);
  border: none;
  padding: 10px 22px;
  cursor: pointer;
  font-family: var(--f-body);
  letter-spacing: 0.08em;
  transition:
    background-color var(--dur-fast) var(--ease-out),
    scale var(--dur-fast) var(--ease-out);
  &:hover { background: var(--primary); }
  &:active { scale: 0.96; }
  &:focus-visible { outline: 2px solid var(--primary); outline-offset: 2px; }
}
</style>
