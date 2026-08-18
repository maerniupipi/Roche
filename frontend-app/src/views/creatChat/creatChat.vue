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
</template>
<script setup lang="ts">
import { ref, computed } from 'vue';
import ContextualGuide from '@/components/ContextualGuide.vue';
import InputField from '@/components/Input-field.vue';
import SuggestedQuestions from '@/components/SuggestedQuestions.vue';
import { createSessions } from "@/api/chat/index";
import { useMenuStore } from '@/stores/menu';
import { useSettingsStore } from '@/stores/settings';
import { useRoute, useRouter } from 'vue-router';
import { MessagePlugin } from 'tdesign-mobile-vue';
import { useI18n } from 'vue-i18n';

const router = useRouter();
const route = useRoute();
const usemenuStore = useMenuStore();
const settingsStore = useSettingsStore();
const { t } = useI18n();

const showChatContextualGuide = computed(() => {
  return route.name === 'globalCreatChat';
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
  usemenuStore.updataMenuChildren(obj);
  usemenuStore.changeIsFirstSession(true);
  usemenuStore.changeFirstQuery(value, mentionedItems, modelId, imageFiles, attachmentFiles);
  router.push(`/platform/chat/${sessionId}`);
}

</script>
<style lang="less" scoped>
.dialogue-wrap {
  flex: 1;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  padding: 0 16px 12px;
  min-height: 0;
}

.dialogue-answers:deep {
  flex: 1;
  display: flex;
  flex-flow: column;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  max-width: 100%;
  gap: 24px;

  .answers-input {
    position: static;
    transform: translateX(0);
  }
}

.dialogue-top {
  margin-top: 44px;
  width: 100%;
}

.dialogue-newSessionTitle,
.dialogue-title {
  display: flex;
  color: var(--td-font-gray-2);
  font-family: var(--app-font-family);
  font-size: 16px;
  text-align: center;
  align-items: center;
  justify-content: center;
  margin-bottom: 0;
  gap: 10px;
  margin-bottom: 6px;
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
  font-size: 24px;
  font-weight: 600;
  color: var(--td-text-color-primary);
  min-height: 44px;
}
</style>
