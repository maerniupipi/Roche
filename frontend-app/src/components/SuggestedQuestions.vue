<template>
  <div ref="containerRef" class="suggested-questions-container"
    :class="{ 'has-questions': loading || questions.length > 0 }">
    <!-- 骨架屏占位 -->
    <div v-if="loading && questions.length === 0" class="suggested-questions-inner">
      <div class="suggested-questions-title" style="margin-bottom: 14px;"><t-skeleton animation="gradient"
          :row-col="[{ width: '120px', height: '15px' }]" /></div>
      <div class="suggested-questions-grid">
        <div v-for="n in skeletonCount" :key="'sq-skel-' + n" class="suggested-question-card sq-card-skeleton">
          <t-skeleton animation="gradient" :row-col="[{ width: '100%', height: '15px', type: 'rect' }]" />
        </div>
      </div>
    </div>
    <transition v-else appear name="sq-slide-fade" mode="out-in" @before-leave="onBeforeLeave"
      @after-leave="onAfterLeave" @enter="onEnter" @after-enter="onQuestionsEntered">
      <div v-if="questions.length > 0" :key="renderKey" class="suggested-questions-inner">
        <div class="suggested-questions-title-row">
          <p class="suggested-questions-caption">
            <img src="@/assets/img/star.svg" alt="star">
            <span class="suggested-questions-title">{{ $t('chat.suggestedQuestions') }}</span>
            <button type="button" class="suggested-questions-refresh" :disabled="loading"
              :title="$t('chat.refreshSuggestedQuestions')" :aria-label="$t('chat.refreshSuggestedQuestions')"
              @click="refresh">
              <t-icon :name="loading ? 'loading' : 'refresh'" :class="{ 'sq-refresh-spin': loading }" />
            </button>
          </p>
        </div>
        <div class="suggested-questions-grid">
          <div v-for="(item, index) in questions" :key="item.question" class="suggested-question-card"
            :class="{ 'sq-card-visible': cardsRevealed }"
            :style="{ transitionDelay: cardsRevealed ? `${index * 50}ms` : '0ms' }" @click="handleClick(item.question)">
            <span class="suggested-question-text">{{ item.question }}</span>
            <span v-if="item.source === 'faq'" class="suggested-question-badge faq">FAQ</span>
          </div>
        </div>
      </div>
    </transition>
  </div>
</template>

<script setup lang="ts">
import { ref, watch, onMounted, nextTick } from 'vue';
import { getSuggestedQuestions } from '@/api/agent/index';
import type { SuggestedQuestion } from '@/api/agent/index';
import { useSettingsStore } from '@/stores/settings';

defineOptions({ name: 'SuggestedQuestions' });

const props = withDefaults(defineProps<{
  /**
   * 是否启用数据获取。
   * 父级在历史未加载完、会话临时不可用等场景下传 false，
   * 组件会跳过首次抓取并在 enabled 重新变为 true 时立即补一次请求。
   */
  enabled?: boolean;
  /** 骨架屏占位卡片数量 */
  skeletonCount?: number;
}>(), {
  enabled: true,
  skeletonCount: 3,
});

const emit = defineEmits<{
  (e: 'select', question: string): void;
}>();

const settingsStore = useSettingsStore();
const questions = ref<SuggestedQuestion[]>([]);
const loading = ref(true);
const cardsRevealed = ref(false);
const renderKey = ref(0);
const containerRef = ref<HTMLElement | null>(null);
let fetchId = 0;
let debounceTimer: ReturnType<typeof setTimeout> | null = null;

// --- 高度平滑过渡钩子 ---
const onBeforeLeave = () => {
  const c = containerRef.value;
  if (!c) return;
  c.style.height = c.offsetHeight + 'px';
  c.style.overflow = 'hidden';
};

const onAfterLeave = () => {
  const c = containerRef.value;
  if (!c) return;
  if (questions.value.length === 0) {
    requestAnimationFrame(() => { c.style.height = '0px'; });
    c.addEventListener('transitionend', () => {
      c.style.height = '';
      c.style.overflow = '';
    }, { once: true });
  }
};

const onEnter = (el: Element) => {
  const c = containerRef.value;
  if (!c) return;
  const startHeight = c.offsetHeight;
  c.style.height = 'auto';
  c.style.overflow = 'hidden';
  const targetHeight = c.offsetHeight;
  c.style.height = startHeight + 'px';
  requestAnimationFrame(() => {
    c.style.height = targetHeight + 'px';
  });
};

const onQuestionsEntered = () => {
  const c = containerRef.value;
  if (c) {
    c.style.height = '';
    c.style.overflow = '';
  }
  nextTick(() => { cardsRevealed.value = true; });
};

const fetchQuestions = async () => {
  const currentId = ++fetchId;
  loading.value = true;
  try {
    const agentId = settingsStore.selectedAgentId;
    if (!agentId) return;
    const res = await getSuggestedQuestions(agentId, settingsStore.getSuggestedQuestionsParams(3));
    if (currentId === fetchId) {
      cardsRevealed.value = false;
      renderKey.value++;
      questions.value = res?.data?.questions || [];
    }
  } catch (err) {
    console.warn('[SuggestedQuestions] Failed to fetch:', err);
    if (currentId === fetchId) {
      questions.value = [];
    }
  } finally {
    if (currentId === fetchId) {
      loading.value = false;
    }
  }
};

const refresh = () => {
  if (!props.enabled) return;
  fetchQuestions();
};

// 防抖包装，切换知识库/文件时300ms内不重复请求
const debouncedFetch = () => {
  if (!props.enabled) return;
  if (debounceTimer) clearTimeout(debounceTimer);
  debounceTimer = setTimeout(() => { fetchQuestions(); }, 300);
};

// 监听 enabled 变化：true → 立即拉取；false → 取消在途请求并清空
watch(
  () => props.enabled, (newEnabled) => {
    if (newEnabled) {
      fetchQuestions();
    } else {
      fetchId++;
      if (debounceTimer) {
        clearTimeout(debounceTimer);
        debounceTimer = null;
      }
      loading.value = false;
      questions.value = [];
      cardsRevealed.value = false;
    }
  },
);

// 监听 Agent / 知识库 / 文件 / 标签 / Skill @mention
watch(
  () => ({
    agentId: settingsStore.selectedAgentId,
    kbs: settingsStore.settings.selectedKnowledgeBases,
    files: settingsStore.settings.selectedFiles,
    tags: settingsStore.settings.selectedTags,
    skills: settingsStore.settings.selectedSkills,
  }),
  debouncedFetch,
  { deep: true },
);

onMounted(() => {
  if (props.enabled) {
    fetchQuestions();
  }
});

const handleClick = (question: string) => {
  emit('select', question);
};
</script>

<style lang="less" scoped>
.suggested-questions-container {
  background: linear-gradient(90deg, #d7ebfb, #fef8f6 53.37%, #f4ecf9);
  margin-top: 36px;
}
</style>