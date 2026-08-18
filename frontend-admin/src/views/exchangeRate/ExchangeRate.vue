<template>
  <div class="exchange-page">
    <header class="exchange-page__header">
      <!-- <h1 class="exchange-page__title">{{ t('exchangeRate.title') }}</h1> -->
      <iconTitle :title="t('exchangeRate.title')" />
    </header>
    <section class="exchange-card">
      <div class="exchange-card-center">
        <div class="exchange-card__labels">
          <label class="exchange-card__label" for="cny-amount">
            <img src="@/assets/img/cnyIcon.svg" alt="CNY"> {{ t('exchangeRate.cny') }}
          </label>
          <div></div>
          <label class="exchange-card__label" for="chf-amount">
            <img src="@/assets/img/chfIcon.svg" alt="CHF"> {{ t('exchangeRate.chf') }}
          </label>
        </div>
        <div class="exchange-card__inputs">
          <div class="exchange-card__input-wrap">
            <t-input id="cny-amount" v-model="cnyAmount" type="number"
              :placeholder="t('exchangeRate.amountPlaceholder')" />
          </div>
          <!-- 中间兑换箭头：用 inline SVG 避免引入额外图标依赖 -->
          <span class="exchange-card__swap" aria-hidden="true">
            <img src="@/assets/img/exchangeIcon.svg" alt="Exchange">
          </span>
          <div class="exchange-card__input-wrap">
            <t-input id="chf-amount" v-model="chfAmount" type="number"
              :placeholder="t('exchangeRate.amountPlaceholder')" />
          </div>
        </div>
        <p v-if="submitError" class="exchange-card__error" role="alert">
          {{ submitError }}
        </p>
      </div>
    </section>
    <footer class="exchange-page-footer">
      <button type="button" class="exchange-page__confirm" :disabled="isConfirmDisabled" @click="handleConfirm">
        {{ t('exchangeRate.confirm') }}
      </button>
    </footer>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { MessagePlugin } from 'tdesign-vue-next'
import iconTitle from '@/components/common/iconTitle.vue'
import {
  getExchangeRate,
  updateExchangeRate,
  type ExchangeRateConfig,
} from '@/api/system'

const { t } = useI18n()

// 两侧金额独立维护，按接口约定两字段一起保存，不做反向换算。
const cnyAmount = ref('')
const chfAmount = ref('')
const isSubmitting = ref(false)
const submitError = ref('')

const rmbValid = computed(() => {
  const v = Number(cnyAmount.value)
  return Number.isFinite(v) && v > 0
})
const chfValid = computed(() => {
  const v = Number(chfAmount.value)
  return Number.isFinite(v) && v > 0
})
const isConfirmDisabled = computed(
  () => isSubmitting.value || !rmbValid.value || !chfValid.value,
)

function applyConfig(cfg: ExchangeRateConfig | null): void {
  if (!cfg) return
  cnyAmount.value = cfg.rmb_amount != null ? String(cfg.rmb_amount) : ''
  chfAmount.value = cfg.chf_amount != null ? String(cfg.chf_amount) : ''
}

async function loadExchangeRate(): Promise<void> {
  try {
    const cfg = await getExchangeRate()
    applyConfig(cfg)
  } catch (err) {
    // 后端无配置或鉴权失败时降级为空表单，不阻塞用户输入
    console.warn('[exchangeRate] load failed', err)
  }
}

async function handleConfirm(): Promise<void> {
  if (isConfirmDisabled.value) return
  submitError.value = ''
  isSubmitting.value = true
  try {
    const cfg = await updateExchangeRate({
      rmb_amount: Number(cnyAmount.value),
      chf_amount: Number(chfAmount.value),
    })
    MessagePlugin.success(t('exchangeRate.submitSuccess'))
  } catch (err: unknown) {
    const msg =
      (err as { message?: string })?.message ||
      t('exchangeRate.submitFailed')
    submitError.value = msg
    MessagePlugin.error({
      content: msg,
      closeBtn: true,
    })
  } finally {
    isSubmitting.value = false
  }
}

onMounted(() => {
  loadExchangeRate()
})
</script>

<style lang="less" scoped>
@import '@/assets/styles/page-shared.less';

.exchange-page__title {
  margin: 0;
  font-size: 16px;
  font-weight: 500;
  color: #191919;
  line-height: 1.5;
}

.exchange-card-center {
  background: rgba(11, 65, 205, 0.04);
  border-radius: 8px;
  flex: 1;
  padding: 24px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.exchange-card__labels {
  display: grid;
  grid-template-columns: 1fr 24px 1fr;
  gap: 30px;
}

.exchange-card__label {
  font-size: 14px;
  font-weight: 500;
  color: #21201f;
  line-height: 1.5;
  display: flex;
  align-items: center;
  gap: 4px;
}

.exchange-card__inputs {
  display: grid;
  grid-template-columns: 1fr 24px 1fr;
  gap: 30px;
  align-items: center;
}

.exchange-card__input-wrap {
  min-width: 0;
}

.exchange-card__swap {
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.exchange-card__error {
  margin: 4px 0 0;
  font-size: 12px;
  line-height: 1.5;
  color: #d54941;
}
</style>
