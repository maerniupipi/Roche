<template>
  <div class="dialogue-wrap">
    <div class="dialogue-answers">
      <div class="dialogue-top">
        <div class="dialogue-newSessionTitle">
          <img class="icon-newSessionTitle" src="@/assets/img/newSessionTitle.svg" alt="newSessionTitle" />
          <span>{{ $t('createChat.newSessionTitle') }}</span>
        </div>
        <div class="dialogue-title">
          <span>{{ $t('createChat.title') }}</span>
        </div>
        <!-- 推荐问题 -->
        <SuggestedQuestions @select="handleSuggestedQuestionClick" />
      </div>
      <InputField ref="inputFieldRef" @send-msg="sendMsg"></InputField>
    </div>
  </div>

  <ContextualGuide tour="chat" :when="showChatContextualGuide" />

  <!-- 知识库编辑器（创建/编辑统一组件） -->
  <KnowledgeBaseEditorModal :visible="uiStore.showKBEditorModal" :mode="uiStore.kbEditorMode"
    :kb-id="uiStore.currentKBId || undefined" :initial-type="uiStore.kbEditorType"
    @update:visible="(val) => val ? null : uiStore.closeKBEditor()" @success="handleKBEditorSuccess" />
</template>
<script setup lang="ts">
import { ref, computed } from 'vue';
import ContextualGuide from '@/components/ContextualGuide.vue';
import InputField from '@/components/Input-field.vue';
import SuggestedQuestions from '@/components/SuggestedQuestions.vue';
import { createSessions } from "@/api/chat/index";
import { useMenuStore } from '@/stores/menu';
import { useSettingsStore } from '@/stores/settings';
import { useUIStore } from '@/stores/ui';
import { useRoute, useRouter } from 'vue-router';
import { MessagePlugin } from 'tdesign-vue-next';
import { useI18n } from 'vue-i18n';
import KnowledgeBaseEditorModal from '@/views/knowledge/KnowledgeBaseEditorModal.vue';
import { useKnowledgeBaseCreationNavigation } from '@/hooks/useKnowledgeBaseCreationNavigation';

const router = useRouter();
const route = useRoute();
const usemenuStore = useMenuStore();
const settingsStore = useSettingsStore();
const uiStore = useUIStore();
const { t } = useI18n();
const { navigateToKnowledgeBaseList } = useKnowledgeBaseCreationNavigation();

const showChatContextualGuide = computed(() => {
  return route.name === 'globalCreatChat' || route.name === 'kbCreatChat';
});

const inputFieldRef = ref();

const handleSuggestedQuestionClick = (question: string) => {
  inputFieldRef.value?.triggerSend(question);
};

const sendMsg = (value: string, modelId: string, mentionedItems: any[], imageFiles: any[] = [], attachmentFiles: any[] = []) => {
  createNewSession(value, modelId, mentionedItems, imageFiles, attachmentFiles);
}

async function createNewSession(value: string, modelId: string, mentionedItems: any[] = [], imageFiles: any[] = [], attachmentFiles: any[] = []) {
    try {
        // Agent configuration is loaded by agent_id when the first message is
        // sent. Knowledge access is always derived from the current user.
        const res = await createSessions({});
        if (res.data && res.data.id) {
            await navigateToSession(res.data.id, value, modelId, mentionedItems, imageFiles, attachmentFiles);
        } else {
            console.error('[createChat] Failed to create session');
            MessagePlugin.error(t('createChat.messages.createFailed'));
        }
    } catch (error) {
        console.error('[createChat] Create session error:', error);
        MessagePlugin.error(t('createChat.messages.createError'));
    }
}

const navigateToSession = async (sessionId: string, value: string, modelId: string, mentionedItems: any[], imageFiles: any[] = [], attachmentFiles: any[] = []) => {
  const now = new Date().toISOString();
  let obj = {
    title: t('createChat.newSessionTitle'),
    path: `chat/${sessionId}`,
    id: sessionId,
    isMore: false,
    isNoTitle: true,
    created_at: now,
    updated_at: now
  };
  usemenuStore.changeIsFirstSession(true);
  usemenuStore.changeFirstQuery(value, mentionedItems, modelId, imageFiles, attachmentFiles);
  router.push(`/platform/chat/${sessionId}`);
}

const handleKBEditorSuccess = (kbId: string) => {
  navigateToKnowledgeBaseList(kbId)
}

</script>
<style lang="less" scoped>
@import '../../components/css/suggested-questions.less';

.dialogue-wrap {
  flex: 1;
  display: flex;
  justify-content: center;
  padding: 12vh 0 20px 20px;
}

.dialogue-answers {
  display: flex;
  flex-flow: column;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  max-width: 800px;
  gap: 24px;

  :deep(.answers-input) {
    position: static;
    transform: translateX(0);
  }
}

.dialogue-top {
  width: 100%;
}

.dialogue-newSessionTitle,
.dialogue-title {
  display: flex;
  color: var(--td-font-gray-2);
  font-family: var(--app-font-family);
  font-size: 16px;
  font-weight: 600;
  align-items: center;
  justify-content: center;
  margin-bottom: 0;
  gap: 10px;
  margin-bottom: 10px;
  min-height: 22px;

  .icon {
    display: flex;
    width: 32px;
    height: 32px;
    justify-content: center;
    align-items: center;
    border-radius: 6px;
    background: var(--td-bg-color-container);
    box-shadow: var(--td-shadow-1);
    margin-right: 12px;

    .logo_img {
      height: 24px;
      width: 24px;
    }
  }
}

.dialogue-newSessionTitle {
  font-size: 28px;
  color: var(--td-text-color-primary);
  min-height: 40px;
}


@keyframes skeletonFadeIn {
  from {
    opacity: 0;
  }

  to {
    opacity: 1;
  }
}

.suggested-questions-container {
  max-width: 800px;
  margin: 40px 0 0;
  padding: 20px 16px;
  border-radius: 8px;
  transition: height 0.35s @suggested-ease;
  background: linear-gradient(90deg, #d7ebfb, #fef8f6 53.37%, #f4ecf9);
  box-sizing: border-box;
}

.suggested-questions-inner {
  animation: skeletonFadeIn 0.3s ease-out;
}

.sq-slide-fade-enter-active {
  transition: opacity 0.35s @suggested-ease, transform 0.35s @suggested-ease;
}

.sq-slide-fade-leave-active {
  transition: opacity 0.15s cubic-bezier(0.4, 0, 1, 1),
    transform 0.15s cubic-bezier(0.4, 0, 1, 1);
}

.sq-slide-fade-enter-from {
  opacity: 0;
  transform: translateY(10px);
}

.sq-slide-fade-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}

.suggested-question-card {
  opacity: 0;
  transform: translateY(8px) scale(0.97);
  transition:
    opacity 0.35s @suggested-ease,
    transform 0.35s @suggested-ease,
    background 0.2s @suggested-ease,
    border-color 0.25s @suggested-ease,
    box-shadow 0.25s @suggested-ease;

  &.sq-card-skeleton {
    opacity: 1;
    transform: none;
  }

  &.sq-card-visible {
    opacity: 1;
    transform: translateY(0) scale(1);
  }

  &:not(.sq-card-skeleton):active {
    transform: scale(0.98);
  }

  &.sq-card-visible:active {
    transform: scale(0.98);
  }
}

@media (max-width: 1250px) and (min-width: 1045px) {
  .answers-input {
    transform: translateX(-329px);
  }

  :deep(.t-textarea__inner) {
    width: 654px !important;
  }
}

@media (max-width: 1045px) {
  .answers-input {
    transform: translateX(-250px);
  }

  :deep(.t-textarea__inner) {
    width: 500px !important;
  }
}

@media (max-width: 750px) {
  .answers-input {
    transform: translateX(-250px);
  }

  :deep(.t-textarea__inner) {
    width: 340px !important;
  }
}

@media (max-width: 600px) {
  .answers-input {
    transform: translateX(-250px);
  }

  :deep(.t-textarea__inner) {
    width: 300px !important;
  }
}
</style>
