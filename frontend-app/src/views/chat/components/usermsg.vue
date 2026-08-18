<template>
  <div class="user_msg_container" ref="containerRef">
    <div class="user_msg">
      {{ content }}
    </div>
    <picturePreview :reviewImg="reviewImg" :reviewUrl="reviewUrl" @closePreImg="closePreImg" />
  </div>
</template>
<script setup>
import { computed, ref, watch, onMounted, nextTick } from "vue";
import { hydrateProtectedFileImages } from '@/utils/security';
import picturePreview from '@/components/picture-preview.vue';
import { useI18n } from 'vue-i18n';

const { t } = useI18n();

const mentionTagClass = (item) => {
  if (item.type === 'kb') return item.kb_type === 'faq' ? 'faq-tag' : 'kb-tag';
  return `${item.type || 'file'}-tag`;
};

const mentionTagIcon = (item) => {
  if (item.type === 'tag') return 'tag';
  if (item.type === 'skill') return 'bookmark';
  return 'file';
};

const props = defineProps({
  content: {
    type: String,
    required: false
  },
  mentioned_items: {
    type: Array,
    required: false,
    default: () => []
  },
  images: {
    type: Array,
    required: false,
    default: () => []
  },
  attachments: {
    type: Array,
    required: false,
    default: () => []
  },
  channel: {
    type: String,
    required: false,
    default: ''
  }
});

const channelLabelMap = {
  web: () => t('chat.channelWeb'),
  api: () => t('chat.channelApi'),
};

const channelLabel = computed(() => {
  if (!props.channel) return '';
  const label = channelLabelMap[props.channel];
  return typeof label === 'function' ? label() : (label || props.channel);
});

const channelClass = computed(() => props.channel ? `channel-${props.channel}` : '');

const containerRef = ref(null);
const hasImages = computed(() => props.images && props.images.length > 0);
const hasAttachments = computed(() => props.attachments && props.attachments.length > 0);

const getAttachmentIcon = (fileNameOrType) => {
  const ext = (fileNameOrType || '').split('.').pop()?.toLowerCase();
  if (['pdf'].includes(ext)) return 'file-pdf';
  if (['doc', 'docx'].includes(ext)) return 'file-word';
  if (['xls', 'xlsx'].includes(ext)) return 'file-excel';
  if (['ppt', 'pptx'].includes(ext)) return 'file-powerpoint';
  if (['txt', 'md'].includes(ext)) return 'file';
  if (['mp3', 'wav', 'm4a', 'flac', 'ogg', 'aac'].includes(ext)) return 'sound';
  return 'file';
};

const getFileExt = (fileName) => {
  return (fileName || '').split('.').pop()?.toUpperCase() || 'FILE';
};

const formatFileSize = (bytes) => {
  if (!bytes) return '';
  if (bytes < 1024) return bytes + ' B';
  if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
  return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
};

const hydrateImages = async () => {
  await nextTick();
  await hydrateProtectedFileImages(containerRef.value);
};

watch(() => props.images, hydrateImages);
onMounted(hydrateImages);

const reviewImg = ref(false);
const reviewUrl = ref('');

const previewImage = (event) => {
  const src = event.target?.src;
  if (src) {
    reviewUrl.value = src;
    reviewImg.value = true;
  }
};

const closePreImg = () => {
  reviewImg.value = false;
  reviewUrl.value = '';
};
</script>
<style scoped lang="less">
@import '../../../components/css/chat-resource-chips.less';

.user_msg_container {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 6px;
  width: 100%;
}

.mentioned_items {
  .chat-mentioned-items(flex-end);
}

.mentioned_tag {
  .chat-mentioned-tag();
}

.user_msg {
  display: flex;
  padding: 8px 12px;
  flex-direction: column;
  justify-content: center;
  align-items: flex-start;
  gap: 4px;
  flex: 1 0 0;
  border-radius: 12px 0 12px 12px;
  background: var(--user-msg-bg);
  margin-left: auto;
  color: var(--td-text-color-primary);
  font-size: 16px;
  line-height: 1.6;
  text-align: left;
  word-break: break-word;
  overflow-wrap: anywhere;
  box-sizing: border-box;
  white-space: pre-wrap;
}

.user_images {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  justify-content: flex-end;
  max-width: 100%;
}

.user_attachments {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  justify-content: flex-end;
  max-width: 100%;
}

.user_attachment_card {
  display: flex;
  flex-direction: row;
  align-items: center;
  gap: 10px;
  padding: 8px 12px;
  border-radius: 8px;
  border: 1px solid var(--td-border-level-1-color, #e7e7e7);
  background: var(--td-bg-color-container, #fff);
  max-width: 260px;
  min-width: 160px;
  cursor: default;

  .attachment_card_icon {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .attachment_card_info {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .attachment_card_name {
    font-size: 13px;
    font-weight: 500;
    color: var(--td-text-color-primary, #333);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .attachment_card_meta {
    font-size: 11px;
    color: var(--td-text-color-secondary, #999);
    white-space: nowrap;
    box-sizing: border-box;
  }
}

.user_image_thumb {
  width: 120px;
  height: 120px;
  object-fit: cover;
  border-radius: 6px;
  cursor: pointer;
  border: 1px solid var(--td-border-level-2-color, #e7e7e7);
  transition: opacity 0.2s;

  &:hover {
    opacity: 0.85;
  }
}

.channel_tag {
  display: inline-flex;
  align-items: center;
  padding: 1px 6px;
  border-radius: 3px;
  font-size: 11px;
  font-weight: 500;
  line-height: 18px;
  background: var(--td-bg-color-secondarycontainer);
  color: var(--td-text-color-placeholder);
  border: 1px solid var(--td-border-level-2-color, #e7e7e7);

  &.channel-web {
    color: var(--td-brand-color);
    background: var(--td-brand-color-light);
    border-color: var(--td-brand-color-2, rgba(0, 82, 217, 0.1));
  }

  &.channel-api {
    color: var(--td-success-color);
    background: var(--td-success-color-1, rgba(0, 168, 112, 0.06));
    border-color: var(--td-success-color-2, rgba(0, 168, 112, 0.15));
  }

}
</style>
