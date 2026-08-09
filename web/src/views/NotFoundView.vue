<script setup lang="ts">
import { useRouter } from 'vue-router'
const router = useRouter()
</script>

<template>
  <div class="page">
    <div class="ticket">
      <div class="mark">RETURN TO SENDER</div>
      <h1>查无此件</h1>
      <p>你访问的地址在档案库中没有登记。</p>
      <button class="back" @click="router.push('/')">回到大厅</button>
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
.ticket {
  border: 1px dashed var(--ink);
  padding: 40px 60px;
  background: var(--paper);
  text-align: center;
  position: relative;
}
.mark {
  border: 2px solid var(--stamp);
  color: var(--stamp);
  display: inline-block;
  padding: 4px 12px;
  font-size: 11px;
  letter-spacing: 0.24em;
  transform: rotate(-4deg);
  margin-bottom: 16px;
}
h1 { font-family: var(--f-display); font-size: 42px; margin: 0 0 8px; }
p { color: var(--dim); }
.back {
  margin-top: 16px;
  background: var(--ink);
  color: var(--paper);
  border: none;
  padding: 10px 20px;
  cursor: pointer;
  font-family: var(--f-body);
  letter-spacing: 0.08em;
}
</style>
