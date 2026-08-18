<template>
  <SettingDrawer :visible="dialogVisible" :title="isEdit ? $t('model.editor.editTitle') : $t('model.editor.addTitle')"
    :description="getModalDescription()" :icon="modelTypeIcon" :confirm-loading="saving"
    @update:visible="(v: boolean) => dialogVisible = v" @confirm="handleConfirm" @cancel="handleCancel">

    <!--
      Footer-left slot: connection-test button lives here so it sits next to
      Save/Cancel — primary actions all aligned along the bottom of the
      drawer. Avoids the "test, then scroll back down to save" dance.
      Mirrors the pattern used in WebSearchSettings' provider drawer.
    -->
    <template v-if="formData.source === 'remote'" #footer-left>
      <t-button variant="outline" @click="checkRemoteAPI" :loading="checking"
        :disabled="!formData.modelName || !formData.baseUrl">
        <template #icon>
          <t-icon v-if="!checking && remoteChecked && remoteAvailable" name="check-circle-filled"
            class="status-icon available" />
          <t-icon v-else-if="!checking && remoteChecked && !remoteAvailable" name="close-circle-filled"
            class="status-icon unavailable" />
        </template>
        {{ checking ? $t('model.editor.testing') : $t('model.editor.testConnection') }}
      </t-button>
      <span v-if="remoteChecked" :class="['footer-test-message', remoteAvailable ? 'success' : 'error']"
        :title="remoteMessage">
        {{ remoteMessage }}
      </span>
    </template>

    <t-form ref="formRef" :data="formData" :rules="rules" layout="vertical">

      <section v-if="!isEdit" class="setting-drawer__section">
        <h4 class="setting-drawer__section-title">{{ $t('model.editor.sectionType') }}</h4>
        <div class="model-type-options" role="radiogroup" :aria-label="$t('model.editor.typeLabel')">
          <button
            v-for="opt in modelTypeChoices"
            :key="opt.value"
            type="button"
            class="model-type-option"
            :class="{ 'is-active': activeModelType === opt.value }"
            role="radio"
            :aria-checked="activeModelType === opt.value"
            @click="selectModelType(opt.value)"
          >
            <t-icon :name="opt.icon" class="model-type-option__icon" />
            <span class="model-type-option__label">{{ opt.label }}</span>
          </button>
        </div>
      </section>

      <!-- Remote API 配置 -->
      <template v-if="formData.source === 'remote'">
        <section class="setting-drawer__section">
          <h4 class="setting-drawer__section-title">{{ $t('model.editor.sectionProvider') }}</h4>

          <!-- 厂商选择器 -->
          <div class="form-item">
            <label class="form-label">{{ $t('model.editor.providerLabel') }}</label>
            <t-select v-model="formData.provider" :placeholder="$t('model.editor.providerPlaceholder')"
              @change="handleProviderChange" :popup-props="{ overlayClassName: 'provider-select-popup' }">
              <!--
                show-overflow-tooltip=false: TDesign 默认在 hover 时给选项浮一个
                完整 label 的小气泡，但这里选项本身就是双行（主名 + 描述），不会
                出现省略，tooltip 只会和已经命中的灰底打架。直接关掉。
              -->
              <t-option v-for="opt in providerOptions" :key="opt.value" :value="opt.value" :label="opt.label"
                :show-overflow-tooltip="false">
                <div class="provider-option">
                  <span class="provider-name">{{ opt.label }}</span>
                  <span class="provider-desc">{{ opt.description }}</span>
                </div>
              </t-option>
            </t-select>
          </div>

          <div class="form-item">
            <label class="form-label required">{{ $t('model.modelName') }}</label>
            <t-input v-model="formData.modelName" :placeholder="getModelNamePlaceholder()" />
          </div>

          <div class="form-item">
            <label class="form-label">{{ $t('model.editor.displayNameLabel') }}</label>
            <t-input v-model="formData.displayName" :placeholder="$t('model.editor.displayNamePlaceholder')" />
            <p class="form-desc">{{ $t('model.editor.displayNameDesc') }}</p>
          </div>

          <div class="form-item">
            <label class="form-label required">{{ $t('model.editor.baseUrlLabel') }}</label>
            <t-input v-model="formData.baseUrl" :placeholder="getBaseUrlPlaceholder()" />
          </div>

          <div class="form-item">
            <label class="form-label">{{
              isLkeapRerank ? $t('model.editor.lkeap.secretIdLabel') : $t('model.editor.apiKeyOptional')
            }}</label>
            <!--
              Edit mode: credentials live behind the /credentials subresource
              of the model — managed by the shared CredentialResource card,
              which now renders an INPUT-LOOKING row (32px tall, same border
              + radius as t-input) so it sits flush with the Base URL field
              above and the 自定义请求头 controls below — no more
              "card inside a card" feel.
              Create mode: the resource doesn't exist yet, so we render a
              plain password input with a leading lock icon and a trailing
              show/hide eye toggle.
            -->
            <CredentialResource v-if="isEdit && props.modelData?.id" :api="credentialApi" :fields="credentialFields"
              :meta="credentialMeta" />
            <t-input v-else v-model="formData.apiKey" :type="showApiKey ? 'text' : 'password'"
              :placeholder="isLkeapRerank ? $t('model.editor.lkeap.secretIdPlaceholder') : apiKeyPlaceholder"
              class="api-key-input" autocomplete="off" spellcheck="false">
              <template #prefix-icon><t-icon name="lock-on" /></template>
              <template #suffix-icon>
                <t-icon
                  :name="showApiKey ? 'browse-off' : 'browse'"
                  class="api-key-toggle"
                  :aria-label="showApiKey ? 'Hide' : 'Show'"
                  @click.stop="showApiKey = !showApiKey"
                />
              </template>
            </t-input>
            <p v-if="isLkeapRerank" class="form-desc">{{ $t('model.editor.lkeap.rerankCredentialHint') }}</p>
          </div>

          <!-- LKEAP Rerank 创建模式：SecretKey（编辑模式由 CredentialResource 管理） -->
          <div v-if="isLkeapRerank && !isEdit" class="form-item">
            <label class="form-label required">{{ $t('model.editor.lkeap.secretKeyLabel') }}</label>
            <t-input v-model="formData.appSecret" type="password"
              :placeholder="$t('model.editor.lkeap.secretKeyPlaceholder')" autocomplete="off" spellcheck="false">
              <template #prefix-icon><t-icon name="lock-on" /></template>
            </t-input>
          </div>

          <div v-if="isLkeapRerank" class="form-item">
            <label class="form-label">{{ $t('model.editor.lkeap.regionLabel') }}</label>
            <t-input v-model="formData.lkeapRegion" :placeholder="$t('model.editor.lkeap.regionPlaceholder')" />
            <p class="form-desc">{{ $t('model.editor.lkeap.regionDesc') }}</p>
          </div>

          <!-- 自定义 HTTP Header（类似 OpenAI Python SDK 的 extra_headers） -->
          <div class="form-item">
            <div class="custom-headers-header">
              <label class="form-label" style="margin-bottom: 0;">{{ $t('model.editor.customHeadersLabel') }}</label>
              <t-button variant="text" size="small" theme="primary" @click="addCustomHeader">
                <template #icon><t-icon name="add" /></template>
                {{ $t('model.editor.customHeadersAdd') }}
              </t-button>
            </div>
            <p class="form-desc custom-headers-desc">{{ $t('model.editor.customHeadersDesc') }}</p>
            <div v-if="formData.customHeaders && formData.customHeaders.length > 0" class="custom-headers-list">
              <div v-for="(item, idx) in formData.customHeaders" :key="idx" class="custom-header-row">
                <t-input v-model="item.key" :placeholder="$t('model.editor.customHeadersKeyPlaceholder')"
                  class="custom-header-key" />
                <t-input v-model="item.value" :placeholder="$t('model.editor.customHeadersValuePlaceholder')"
                  class="custom-header-value" />
                <t-button variant="text" shape="square" size="small" class="custom-header-remove"
                  @click="removeCustomHeader(idx)" :aria-label="$t('common.delete')">
                  <t-icon name="close" />
                </t-button>
              </div>
            </div>
          </div>

          <!--
            Connection test action moved to the drawer footer (footer-left
            slot above) so primary actions live in one row at the bottom.
          -->
        </section>
      </template>

      <!-- Section 3 — 高级选项（仅在有内容时渲染，避免空 section 出现底部分隔线） -->
      <section v-if="activeModelType === 'embedding' || activeModelType === 'chat'" class="setting-drawer__section">
        <h4 class="setting-drawer__section-title">{{ $t('model.editor.sectionAdvanced') }}</h4>

        <!-- Embedding 专用：维度 -->
        <div v-if="activeModelType === 'embedding'" class="form-item">
          <label class="form-label">{{ $t('model.editor.dimensionLabel') }}</label>
          <div class="dimension-control">
            <t-input v-model.number="formData.dimension" type="number" :min="128" :max="4096"
              :placeholder="$t('model.editor.dimensionPlaceholder')"
              :disabled="!formData.supportsDimensionOverride" />
          </div>
        </div>

        <div v-if="activeModelType === 'embedding'" class="form-item">
          <label class="form-label">{{ $t('model.editor.dimensionOverrideLabel') }}</label>
          <div class="vision-toggle">
            <t-switch v-model="formData.supportsDimensionOverride" />
            <span class="form-desc form-desc--inline">{{ $t('model.editor.dimensionOverrideDesc') }}</span>
          </div>
        </div>

        <!-- Chat: supports vision toggle (VLLM models are inherently multimodal) -->
        <div v-if="activeModelType === 'chat'" class="form-item">
          <label class="form-label">{{ $t('model.editor.supportsVisionLabel') }}</label>
          <div class="vision-toggle">
            <t-switch v-model="formData.supportsVision" />
            <span class="form-desc form-desc--inline">{{ $t('model.editor.supportsVisionDesc') }}</span>
          </div>
        </div>

        <!-- Chat + 远程 API：思考模式参数格式 -->
        <div v-if="showThinkingControlField" class="form-item">
          <label class="form-label">{{ $t('model.editor.thinkingControlLabel') }}</label>
          <t-select
            v-model="formData.thinkingControl"
            :key="`thinking-${formData.id}-${formData.thinkingControl}`"
            :popup-props="{ overlayClassName: 'thinking-control-select-popup' }"
            @change="onThinkingControlManualPick"
          >
            <t-option
              v-for="opt in thinkingControlOptions"
              :key="opt.value"
              :value="opt.value"
              :label="opt.label"
              :show-overflow-tooltip="false"
            >
              <div class="thinking-control-option">
                <span class="thinking-control-option__title">{{ opt.label }}</span>
                <span class="thinking-control-option__hint">{{ opt.hint }}</span>
              </div>
            </t-option>
          </t-select>
          <p class="form-desc">{{ $t('model.editor.thinkingControlDesc') }}</p>
        </div>
      </section>

    </t-form>
  </SettingDrawer>
</template>

<script setup lang="ts">
import { ref, watch, computed, nextTick } from 'vue'
import { MessagePlugin } from 'tdesign-vue-next'
import { checkRemoteModel, testEmbeddingModel, checkRerankModel, checkASRModel, listModelProviders, type ModelProviderOption } from '@/api/initialization'
import {
  putModelCredentials,
  deleteModelCredentialField,
  type ModelCredentialField,
} from '@/api/model'
import { useI18n } from 'vue-i18n'
import {
  defaultThinkingControl,
  resolveThinkingControl,
  type ThinkingControlValue,
} from '@/utils/thinkingControl'
import SettingDrawer from '@/components/settings/SettingDrawer.vue'
import CredentialResource, {
  type CredentialFieldDef,
  type CredentialResourceApi,
} from '@/components/credentials/CredentialResource.vue'

interface CustomHeaderItem {
  key: string
  value: string
}

interface ModelFormData {
  id: string
  name: string
  source: 'remote'
  provider?: string // Provider identifier: openai, aliyun, zhipu, generic, etc.
  modelName: string
  displayName?: string
  baseUrl?: string
  apiKey?: string
  dimension?: number
  supportsDimensionOverride?: boolean
  interfaceType?: 'openai'
  isDefault: boolean
  supportsVision?: boolean
  /** extra_config.thinking_control — how agent thinking on/off maps to API fields. */
  thinkingControl?: string
  // 自定义 HTTP 请求头（类似 OpenAI Python SDK 的 extra_headers）
  customHeaders?: CustomHeaderItem[]
  /** LKEAP Rerank：腾讯云 SecretKey（创建时写入 app_secret） */
  appSecret?: string
  /** LKEAP Rerank：地域，如 ap-guangzhou */
  lkeapRegion?: string
}

type EditorModelType = 'chat' | 'embedding' | 'rerank' | 'vllm' | 'asr'

interface Props {
  visible: boolean
  modelType: EditorModelType
  modelData?: ModelFormData | null
}

const { t, te } = useI18n()

const props = withDefaults(defineProps<Props>(), {
  visible: false,
  modelData: null
})

const emit = defineEmits<{
  'update:visible': [value: boolean]
  'confirm': [data: ModelFormData & { modelType?: EditorModelType }]
}>()

const draftModelType = ref<EditorModelType>(props.modelType)

const isEdit = computed(() => !!props.modelData)

const activeModelType = computed(() => (
  isEdit.value ? props.modelType : draftModelType.value
))

const modelTypeChoices = computed(() => ([
  { value: 'chat' as const, label: t('modelSettings.typeShort.chat'), icon: 'chat' },
  { value: 'embedding' as const, label: t('modelSettings.typeShort.embedding'), icon: 'chart-bubble' },
  { value: 'rerank' as const, label: t('modelSettings.typeShort.rerank'), icon: 'filter-sort' },
  { value: 'vllm' as const, label: t('modelSettings.typeShort.vllm'), icon: 'image' },
  { value: 'asr' as const, label: t('modelSettings.typeShort.asr'), icon: 'sound' },
]))

// API 返回的 Provider 列表
const apiProviderOptions = ref<ModelProviderOption[]>([])
const loadingProviders = ref(false)

// 硬编码的后备 Provider 配置 (当 API 不可用时使用)
const fallbackProviderOptions = computed(() => [
  {
    value: 'openai',
    label: t('model.editor.providers.openai.label'),
    defaultUrls: {
      chat: 'https://api.openai.com/v1',
      embedding: 'https://api.openai.com/v1',
      rerank: 'https://api.openai.com/v1',
      vllm: 'https://api.openai.com/v1',
      asr: 'https://api.openai.com/v1'
    },
    description: t('model.editor.providers.openai.description'),
    modelTypes: ['chat', 'embedding', 'vllm', 'asr']
  },
  {
    value: 'azure_openai',
    label: t('model.editor.providers.azure_openai.label'),
    defaultUrls: {
      chat: 'https://{resource}.openai.azure.com',
      embedding: 'https://{resource}.openai.azure.com',
      vllm: 'https://{resource}.openai.azure.com',
      asr: 'https://{resource}.openai.azure.com'
    },
    description: t('model.editor.providers.azure_openai.description'),
    modelTypes: ['chat', 'embedding', 'vllm', 'asr']
  },
  {
    value: 'aliyun',
    label: t('model.editor.providers.aliyun.label'),
    defaultUrls: {
      chat: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
      embedding: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
      rerank: 'https://dashscope.aliyuncs.com/api/v1/services/rerank/text-rerank/text-rerank',
      vllm: 'https://dashscope.aliyuncs.com/compatible-mode/v1'
    },
    description: t('model.editor.providers.aliyun.description'),
    modelTypes: ['chat', 'embedding', 'rerank', 'vllm']
  },
  {
    value: 'zhipu',
    label: t('model.editor.providers.zhipu.label'),
    defaultUrls: {
      chat: 'https://open.bigmodel.cn/api/paas/v4',
      embedding: 'https://open.bigmodel.cn/api/paas/v4/embeddings',
      vllm: 'https://open.bigmodel.cn/api/paas/v4'
    },
    description: t('model.editor.providers.zhipu.description'),
    modelTypes: ['chat', 'embedding', 'vllm']
  },
  {
    value: 'openrouter',
    label: t('model.editor.providers.openrouter.label'),
    defaultUrls: {
      chat: 'https://openrouter.ai/api/v1',
      embedding: 'https://openrouter.ai/api/v1'
    },
    description: t('model.editor.providers.openrouter.description'),
    modelTypes: ['chat', 'embedding']
  },
  {
    value: 'gemini',
    label: t('model.editor.providers.gemini.label'),
    defaultUrls: {
      chat: 'https://generativelanguage.googleapis.com/v1beta/openai',
      embedding: 'https://generativelanguage.googleapis.com/v1beta'
    },
    description: t('model.editor.providers.gemini.description'),
    modelTypes: ['chat', 'embedding']
  },
  {
    value: 'siliconflow',
    label: t('model.editor.providers.siliconflow.label'),
    defaultUrls: {
      chat: 'https://api.siliconflow.cn/v1',
      embedding: 'https://api.siliconflow.cn/v1',
      rerank: 'https://api.siliconflow.cn/v1'
    },
    description: t('model.editor.providers.siliconflow.description'),
    modelTypes: ['chat', 'embedding', 'rerank']
  },
  {
    value: 'jina',
    label: t('model.editor.providers.jina.label'),
    defaultUrls: {
      embedding: 'https://api.jina.ai/v1',
      rerank: 'https://api.jina.ai/v1'
    },
    description: t('model.editor.providers.jina.description'),
    modelTypes: ['embedding', 'rerank']
  },
  {
    value: 'nvidia',
    label: t('model.editor.providers.nvidia.label'),
    defaultUrls: {
      chat: 'https://integrate.api.nvidia.com/v1',
      embedding: 'https://integrate.api.nvidia.com/v1',
      rerank: 'https://ai.api.nvidia.com/v1/retrieval/nvidia/reranking',
      vllm: 'https://integrate.api.nvidia.com/v1',
    },
    description: t('model.editor.providers.nvidia.description'),
    modelTypes: ['chat', 'embedding', 'rerank', 'vllm']
  },
  {
    value: 'novita',
    label: t('model.editor.providers.novita.label'),
    defaultUrls: {
      chat: 'https://api.novita.ai/openai/v1',
      embedding: 'https://api.novita.ai/openai/v1',
      vllm: 'https://api.novita.ai/openai/v1',
    },
    description: t('model.editor.providers.novita.description'),
    modelTypes: ['chat', 'embedding', 'vllm']
  },
  {
    value: 'generic',
    label: t('model.editor.providers.generic.label'),
    defaultUrls: {},
    description: t('model.editor.providers.generic.description'),
    modelTypes: ['chat', 'embedding', 'rerank', 'vllm', 'asr']
  },
])

// 从 API 获取 Provider 列表
const loadProviders = async () => {
  loadingProviders.value = true
  try {
    const providers = await listModelProviders(activeModelType.value)
    if (providers.length > 0) {
      apiProviderOptions.value = providers
    }
  } catch (error) {
    console.error('Failed to load providers from API, using fallback', error)
  } finally {
    loadingProviders.value = false
  }
}

// 根据当前模型类型过滤的 Provider 列表
// API 返回的 defaultUrls/modelTypes 数据优先，但 label/description 使用 i18n
const providerOptions = computed(() => {
  // API 数据可用时，用 API 的结构数据 + i18n 的显示文本
  if (apiProviderOptions.value.length > 0) {
    return apiProviderOptions.value.map(p => ({
      ...p,
      label: te(`model.editor.providers.${p.value}.label`)
        ? t(`model.editor.providers.${p.value}.label`)
        : p.label,
      description: te(`model.editor.providers.${p.value}.description`)
        ? t(`model.editor.providers.${p.value}.description`)
        : p.description,
    }))
  }
  // 回退到硬编码值，按 modelTypes 过滤
  return fallbackProviderOptions.value.filter(p =>
    p.modelTypes.includes(activeModelType.value)
  )
})

const dialogVisible = computed({
  get: () => props.visible,
  set: (val) => emit('update:visible', val)
})

const showThinkingControlField = computed(() =>
  activeModelType.value === 'chat' && formData.value.source === 'remote',
)

const resolvedThinkingControl = (): ThinkingControlValue =>
  defaultThinkingControl(
    formData.value.provider || '',
    formData.value.modelName || '',
  )

/** 用户是否手动改过思考参数格式（改过则不再自动覆盖，直到换服务商） */
const thinkingControlManual = ref(false)
/** 正在从 modelData 灌入表单，忽略厂商/来源控件的程序化 change 副作用 */
const hydratingForm = ref(false)

const onThinkingControlManualPick = () => {
  thinkingControlManual.value = true
}

const syncThinkingControlToForm = (force = false) => {
  if (!showThinkingControlField.value) return
  if (!force && !isEdit.value && thinkingControlManual.value) return
  formData.value.thinkingControl = resolvedThinkingControl()
}

const applyThinkingControlFromModelData = () => {
  if (!props.modelData || activeModelType.value !== 'chat' || formData.value.source !== 'remote') return
  thinkingControlManual.value = !!props.modelData.thinkingControl
  formData.value.thinkingControl = resolveThinkingControl(
    props.modelData.thinkingControl,
    formData.value.provider || props.modelData.provider || '',
    formData.value.modelName || props.modelData.modelName || '',
  )
}

const thinkingControlOptions = computed(() => {
  const keys = ['none', 'chatTemplateKwargs', 'enableThinking', 'thinkingType'] as const
  const values = ['none', 'chat_template_kwargs', 'enable_thinking', 'thinking_type'] as const
  return keys.map((key, i) => ({
    value: values[i],
    label: t(`model.editor.thinkingControl.${key}.label`),
    hint: t(`model.editor.thinkingControl.${key}.hint`),
  }))
})

// Header icon for the SettingDrawer — uses the same TDesign icon name table
// as the model card list, so the drawer's leading badge visually matches the
// card the user just clicked on.
const modelTypeIcon = computed(() => {
  const map: Record<string, string> = {
    chat: 'chat',
    embedding: 'chart-bubble',
    rerank: 'filter-sort',
    vllm: 'image',
    asr: 'sound',
  }
  return map[activeModelType.value] || 'setting'
})

const isLkeapRerank = computed(
  () => activeModelType.value === 'rerank' && formData.value.provider === 'lkeap',
)

// Credential resource binding for the shared <CredentialResource> component.
const credentialFields = computed<CredentialFieldDef<ModelCredentialField>[]>(() => {
  const fields: CredentialFieldDef<ModelCredentialField>[] = [
    {
      key: 'api_key',
      label: (isLkeapRerank.value
        ? t('model.editor.lkeap.secretIdLabel')
        : t('model.editor.apiKeyOptional')) as string,
    },
  ]
  if (isLkeapRerank.value) {
    fields.push({ key: 'app_secret', label: t('model.editor.lkeap.secretKeyLabel') as string })
  }
  return fields
})

const credentialApi = computed<CredentialResourceApi<ModelCredentialField>>(() => {
  const id = props.modelData?.id ?? ''
  return {
    save: async (patch) => {
      const meta = await putModelCredentials(id, patch)
      return meta.fields
    },
    remove: async (field) => {
      await deleteModelCredentialField(id, field)
    },
  }
})

// Initial credential metadata. ModelSettings.convertToLegacyFormat
// preserves `credentials` from the main ListModels response so the card
// renders the correct "Configured" state on dialog open.
const credentialMeta = computed(() => (props.modelData as any)?.credentials ?? {
  api_key: { configured: false },
  app_secret: { configured: false },
})

// Placeholder hint for the create-mode API key input. Edit mode replaces
// this input entirely with a <CredentialResource> card.
const apiKeyPlaceholder = computed(() => t('model.editor.apiKeyPlaceholder'))

const formRef = ref()
const saving = ref(false)
// Toggles the create-mode API key input between masked and plain text. Lets
// the user proofread a freshly pasted secret without losing the password
// affordance for everyday use. Reset every time the drawer closes (see
// reset block in the visible watcher) so we never leak the previous value
// across editor sessions.
const showApiKey = ref(false)
const checking = ref(false)
const remoteChecked = ref(false)
const remoteAvailable = ref(false)
const remoteMessage = ref('')
const formData = ref<ModelFormData>({
  id: '',
  name: '',
  source: 'remote',
  provider: 'generic',
  modelName: '',
  displayName: '',
  baseUrl: '',
  apiKey: '',
  dimension: undefined,
  supportsDimensionOverride: false,
  interfaceType: 'openai',
  isDefault: false,
  supportsVision: false,
  thinkingControl: defaultThinkingControl('generic', ''),
  customHeaders: [],
  appSecret: '',
  lkeapRegion: 'ap-guangzhou',
})

const rules = computed(() => ({
  modelName: [
    { required: true, message: t('model.editor.validation.modelNameRequired') },
    {
      validator: (val: string) => {
        if (!val || !val.trim()) {
          return { result: false, message: t('model.editor.validation.modelNameEmpty') }
        }
        if (val.trim().length > 100) {
          return { result: false, message: t('model.editor.validation.modelNameMax') }
        }
        return { result: true }
      },
      trigger: 'blur'
    }
  ],
  baseUrl: [
    {
      required: true,
      message: t('model.editor.validation.baseUrlRequired'),
      trigger: 'blur'
    },
    {
      validator: (val: string) => {
        if (!val || !val.trim()) {
          return { result: false, message: t('model.editor.validation.baseUrlEmpty') }
        }
        // 简单的 URL 格式校验
        try {
          new URL(val.trim())
          return { result: true }
        } catch {
          return { result: false, message: t('model.editor.validation.baseUrlInvalid') }
        }
      },
      trigger: 'blur'
    }
  ]
}))

// 获取弹窗描述文字
const getModalDescription = () => {
  const key = `model.editor.description.${activeModelType.value}` as const
  return t(key) || t('model.editor.description.default')
}

// 获取模型名称占位符
const getModelNamePlaceholder = () => {
  if (activeModelType.value === 'vllm') {
    return t('model.editor.modelNamePlaceholder.remoteVllm')
  }
  if (activeModelType.value === 'asr') {
    return t('model.editor.modelNamePlaceholder.remoteAsr')
  }
  return t('model.editor.modelNamePlaceholder.remote')
}

const getBaseUrlPlaceholder = () => {
  if (activeModelType.value === 'vllm') {
    return t('model.editor.baseUrlPlaceholderVllm')
  }
  if (activeModelType.value === 'asr') {
    return t('model.editor.baseUrlPlaceholderAsr')
  }
  return t('model.editor.baseUrlPlaceholder')
}

const lastOpenedModelId = ref<string | null>(null)

const selectModelType = async (type: EditorModelType) => {
  if (isEdit.value || draftModelType.value === type) return
  draftModelType.value = type

  if (type === 'rerank') {
    formData.value.source = 'remote'
  }
  if (type !== 'embedding') {
    formData.value.dimension = undefined
    formData.value.supportsDimensionOverride = false
  }
  if (type !== 'chat') {
    formData.value.supportsVision = false
    thinkingControlManual.value = false
  }
  remoteChecked.value = false
  remoteAvailable.value = false
  remoteMessage.value = ''

  await loadProviders()
  const supported = providerOptions.value.some(p => p.value === formData.value.provider)
  if (!supported) {
    formData.value.provider = 'generic'
    formData.value.baseUrl = ''
  } else {
    handleProviderChange(formData.value.provider || 'generic')
  }
  if (showThinkingControlField.value && !isEdit.value) {
    thinkingControlManual.value = false
    syncThinkingControlToForm(true)
  }
}

// 监听 visible 变化，初始化表单
watch(() => props.visible, (val) => {
  if (val) {
    // 从 API 加载 Model Provider 列表
    loadProviders()

    // 每次打开都清理上一次遗留的校验/检测结果，避免编辑别的模型时
    // 直接显示上一次的“连接成功”
    remoteChecked.value = false
    remoteAvailable.value = false
    remoteMessage.value = ''

    const currentId = props.modelData?.id ?? null
    draftModelType.value = props.modelType

    hydratingForm.value = true
    try {
      if (props.modelData) {
        // 编辑：始终用最新的 modelData 覆盖。apiKey field is left blank — in
        // edit mode the credential is owned by the <CredentialResource> card,
        // not by this form's apiKey field.
        formData.value = {
          ...props.modelData,
          apiKey: '',
          customHeaders: Array.isArray(props.modelData.customHeaders)
            ? props.modelData.customHeaders.map(h => ({ key: h.key, value: h.value }))
            : [],
        }
        applyThinkingControlFromModelData()
      } else if (lastOpenedModelId.value !== null || !formData.value.id) {
        // 上次是编辑某个模型，或第一次新增 → 重置成空白
        resetForm()
      }
      // 否则：连续两次"新增"打开（中间是点遮罩/ESC 关闭的）→ 保留上次填写

      lastOpenedModelId.value = currentId

      if (activeModelType.value === 'rerank') {
        formData.value.source = 'remote'
      }

      if (showThinkingControlField.value && !isEdit.value) {
        thinkingControlManual.value = false
        syncThinkingControlToForm(true)
      }
    } finally {
      nextTick(() => {
        hydratingForm.value = false
      })
    }
  }
})

// 重置表单
const resetForm = () => {
  thinkingControlManual.value = false
  formData.value = {
    id: generateId(),
    name: '', // 保留字段但不使用，保存时用 modelName
    source: 'remote',
    provider: 'generic',
    modelName: '',
    displayName: '',
    baseUrl: '',
    apiKey: '',
    dimension: undefined, // 默认不填，让用户手动输入或通过检测按钮获取
    supportsDimensionOverride: false,
    interfaceType: undefined,
    isDefault: false,
    supportsVision: false,
    thinkingControl: defaultThinkingControl('generic', ''),
    customHeaders: [],
    appSecret: '',
    lkeapRegion: 'ap-guangzhou',
  }
  remoteChecked.value = false
  remoteAvailable.value = false
  remoteMessage.value = ''
  showApiKey.value = false
}

// 处理厂商选择变化 (自动填充默认 URL)
const handleProviderChange = (value: string) => {
  const provider = providerOptions.value.find(opt => opt.value === value)
  if (provider && provider.defaultUrls) {
    // 根据当前模型类型获取对应的默认 URL
    const defaultUrl = provider.defaultUrls[activeModelType.value]
    if (defaultUrl) {
      formData.value.baseUrl = defaultUrl
    }
    if (value === 'lkeap' && activeModelType.value === 'rerank' && !formData.value.modelName?.trim()) {
      formData.value.modelName = 'lke-reranker-base'
    }
    // 重置校验状态
    remoteChecked.value = false
    remoteAvailable.value = false
    remoteMessage.value = ''
  }
  if (hydratingForm.value) return
  if (activeModelType.value !== 'chat' || formData.value.source !== 'remote') return
  if (!isEdit.value) {
    thinkingControlManual.value = false
    syncThinkingControlToForm(true)
    return
  }
  // 编辑时仅用户主动换厂商才跟随默认
  thinkingControlManual.value = false
  syncThinkingControlToForm(true)
}

watch(
  () => [formData.value.source, formData.value.provider, formData.value.modelName] as const,
  ([source, provider, modelName], [prevSource, prevProvider, prevModelName]) => {
    if (hydratingForm.value || isEdit.value) return
    if (activeModelType.value !== 'chat' || source !== 'remote') return
    if (source === prevSource && provider === prevProvider && modelName === prevModelName) return

    const providerChanged = provider !== prevProvider

    if (providerChanged) {
      thinkingControlManual.value = false
      syncThinkingControlToForm(true)
      return
    }
    if (!thinkingControlManual.value) {
      syncThinkingControlToForm(true)
      return
    }
    const prevDefault = defaultThinkingControl(prevProvider || '', prevModelName || '')
    if (formData.value.thinkingControl === prevDefault) {
      syncThinkingControlToForm(true)
    }
  },
)

// 监听来源变化，重置校验状态（已合并到下面的 watch）

// 生成唯一ID
const generateId = () => {
  return `model_${Date.now()}_${Math.random().toString(36).substr(2, 9)}`
}

// 自定义 HTTP Header 编辑
const addCustomHeader = () => {
  if (!Array.isArray(formData.value.customHeaders)) {
    formData.value.customHeaders = []
  }
  formData.value.customHeaders.push({ key: '', value: '' })
}

const removeCustomHeader = (idx: number) => {
  if (!Array.isArray(formData.value.customHeaders)) return
  formData.value.customHeaders.splice(idx, 1)
}

// 过滤后的模型列表
const checkRemoteAPI = async () => {
  if (!formData.value.modelName || !formData.value.baseUrl) {
    MessagePlugin.warning(t('model.editor.fillModelAndUrl'))
    return
  }

  checking.value = true
  remoteChecked.value = false
  remoteMessage.value = ''

  try {
    let result: any

    // 把表单里 Key-Value 数组形式的自定义 Header 转成后端期望的 map。
    // 跟 ModelSettings.vue 保存时一致，空行自动丢弃，保证测试连接与真正保存后的
    // 生产调用使用完全相同的 Header 集合。
    const customHeaders: Record<string, string> = {}
    if (Array.isArray(formData.value.customHeaders)) {
      for (const item of formData.value.customHeaders) {
        const key = (item?.key ?? '').trim()
        const value = (item?.value ?? '').trim()
        if (key && value) customHeaders[key] = value
      }
    }
    // 只在非空时带上字段，避免在 URL query / 日志里出现空对象
    const headerPayload = Object.keys(customHeaders).length > 0
      ? { customHeaders }
      : {}

    // 根据模型类型调用不同的校验接口
    // 编辑模式下 apiKey 由 <CredentialResource> 独立管理、不在 formData 里。
    // 把 modelId 透传给后端，让它在 apiKey 为空时自动用存储的解密值兜底，
    // 避免出现"测试连接没带 apiKey 直接失败"的情况。
    const idPayload = isEdit.value && props.modelData?.id
      ? { modelId: props.modelData.id as string }
      : {}

    switch (activeModelType.value) {
      case 'chat':
        // 对话模型（KnowledgeQA）
        result = await checkRemoteModel({
          modelName: formData.value.modelName,
          baseUrl: formData.value.baseUrl || '',
          apiKey: formData.value.apiKey || '',
          provider: formData.value.provider,
          ...idPayload,
          ...headerPayload,
        })
        break

      case 'embedding':
        // Embedding 模型
        result = await testEmbeddingModel({
          source: 'remote',
          modelName: formData.value.modelName,
          baseUrl: formData.value.baseUrl || '',
          apiKey: formData.value.apiKey || '',
          dimension: formData.value.dimension,
          supportsDimensionOverride: formData.value.supportsDimensionOverride ?? false,
          provider: formData.value.provider,
          ...idPayload,
          ...headerPayload,
        })
        // 如果测试成功且返回了维度，自动填充
        if (result.available && result.dimension) {
          formData.value.dimension = result.dimension
          MessagePlugin.info(t('model.editor.remoteDimensionDetected', { value: result.dimension }))
        }
        break

      case 'rerank': {
        const lkeapExtra = isLkeapRerank.value
          ? {
              extraConfig: {
                region: (formData.value.lkeapRegion || 'ap-guangzhou').trim(),
              },
              ...(formData.value.appSecret?.trim()
                ? { appSecret: formData.value.appSecret.trim() }
                : {}),
            }
          : {}
        result = await checkRerankModel({
          modelName: formData.value.modelName,
          baseUrl: formData.value.baseUrl || '',
          apiKey: formData.value.apiKey || '',
          provider: formData.value.provider,
          ...idPayload,
          ...headerPayload,
          ...lkeapExtra,
        })
        break
      }

      case 'vllm':
        // VLLM 模型（多模态）
        // VLLM 使用 checkRemoteModel 进行基础连接测试
        result = await checkRemoteModel({
          modelName: formData.value.modelName,
          baseUrl: formData.value.baseUrl || '',
          apiKey: formData.value.apiKey || '',
          provider: formData.value.provider,
          ...idPayload,
          ...headerPayload,
        })
        break

      case 'asr':
        // ASR 模型（语音识别）— 使用专用的 ASR 测试接口（/v1/audio/transcriptions）
        result = await checkASRModel({
          modelName: formData.value.modelName,
          baseUrl: formData.value.baseUrl || '',
          apiKey: formData.value.apiKey || '',
          provider: formData.value.provider,
          ...idPayload,
          ...headerPayload,
        })
        break

      default:
        MessagePlugin.error(t('model.editor.unsupportedModelType'))
        return
    }

    remoteChecked.value = true
    remoteAvailable.value = result.available || false
    // 之前这里把 backend 的错误 message 只丢到 console.debug，用户只能
    // 看到通用的 "连接失败" toast，根本看不出是 401 / 404 / 模型不存在
    // 还是别的什么。改成：成功时用 i18n 通用提示；失败时直接展示后端
    // 给到的具体原因（已经在后端 classifyConnectionError 中包了一层
    // 易读的中文 hint + 原始 SDK 报错），方便排查。
    if (result.available) {
      remoteMessage.value = t('model.editor.connectionSuccess')
      MessagePlugin.success(remoteMessage.value)
    } else {
      remoteMessage.value = result.message || t('model.editor.connectionFailed')
      console.debug('Backend message:', result.message)
      MessagePlugin.error(remoteMessage.value)
    }
  } catch (error: any) {
    console.error('Remote API check failed:', error)
    remoteChecked.value = true
    remoteAvailable.value = false
    // 后端 4xx/5xx（如 SSRF 校验失败）会走到这里。axios 拦截器把后端
    // { error: { message: "..." } } 提到了 error.message，里面已经包含
    // 易读 hint + 原因，直接展示出来，比通用 "请检查配置" 有用得多。
    remoteMessage.value = error?.message || t('model.editor.connectionConfigError')
    MessagePlugin.error(remoteMessage.value)
  } finally {
    checking.value = false
  }
}

// 确认保存
const handleConfirm = async () => {
  try {
    // 手动校验必填字段
    if (!formData.value.modelName || !formData.value.modelName.trim()) {
      MessagePlugin.warning(t('model.editor.validation.modelNameRequired'))
      return
    }

    if (formData.value.modelName.trim().length > 100) {
      MessagePlugin.warning(t('model.editor.validation.modelNameMax'))
      return
    }

    if (formData.value.source === 'remote') {
      if (!formData.value.baseUrl || !formData.value.baseUrl.trim()) {
        MessagePlugin.warning(t('model.editor.remoteBaseUrlRequired'))
        return
      }

      // 校验 Base URL 格式
      try {
        new URL(formData.value.baseUrl.trim())
      } catch {
        MessagePlugin.warning(t('model.editor.validation.baseUrlInvalid'))
        return
      }
    }

    // 执行表单验证
    await formRef.value?.validate()

    // Credential removal in edit mode is handled inline by the
    // CredentialResource card (it confirms + DELETEs to /credentials), so
    // the main save flow no longer needs to confirm or handle clear flags.

    saving.value = true

    // 如果是新增且没有 id，生成一个
    if (!formData.value.id) {
      formData.value.id = generateId()
    }

    emit('confirm', {
      ...formData.value,
      ...(isEdit.value ? {} : { modelType: activeModelType.value }),
    })
    dialogVisible.value = false
    // 保存成功后重置草稿，下次打开新增模型时是空白
    resetForm()
    lastOpenedModelId.value = null
    // 移除此处的成功提示，由父组件统一处理
  } catch (error) {
    console.error('表单验证失败:', error)
  } finally {
    saving.value = false
  }
}

// 监听模型选择变化（处理下载逻辑和自动维度检测提示）
const handleCancel = () => {
  resetForm()
  lastOpenedModelId.value = null
  dialogVisible.value = false
}
</script>

<style lang="less" scoped>
// 原生 t-form-item 容器置空（本组件使用自定义 .form-item + 手写 label）
:deep(.t-form) {
  .t-form-item {
    display: none;
  }
}

// 表单项样式
.form-item {
  // No bottom margin — vertical rhythm is owned by the parent
  // .setting-drawer__section's `gap`. That keeps the spacing inside a section
  // tight and the gap between sections visually distinct.
  margin-bottom: 0;
}

.form-label {
  display: block;
  margin-bottom: 6px;
  font-size: 13px;
  font-weight: 500;
  color: var(--td-text-color-primary);
  line-height: 1.4;

  // TDesign-style required marker: leading asterisk before the label text,
  // matching the rest of the app's <t-form-item required ...> appearance.
  &.required::before {
    content: '*';
    color: var(--td-error-color);
    margin-right: 4px;
    font-weight: 500;
    line-height: 1;
  }
}

.model-type-options {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.model-type-option {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  min-height: 32px;
  border: 1px solid var(--td-component-stroke);
  border-radius: 8px;
  background: var(--td-bg-color-container);
  color: var(--td-text-color-secondary);
  font-size: 13px;
  line-height: 1.4;
  cursor: pointer;
  transition: border-color 0.15s ease, color 0.15s ease, background 0.15s ease;

  &__icon {
    font-size: 15px;
    flex-shrink: 0;
  }

  &__label {
    white-space: nowrap;
  }

  &:hover:not(.is-active) {
    border-color: var(--td-brand-color-3, var(--td-brand-color));
    color: var(--td-text-color-primary);
  }

  &.is-active {
    border-color: var(--td-brand-color);
    background: color-mix(in srgb, var(--td-brand-color) 10%, transparent);
    color: var(--td-brand-color);
    font-weight: 500;
  }

  &:focus-visible {
    outline: 2px solid var(--td-brand-color);
    outline-offset: 2px;
  }
}

// 输入框样式：只在最外层 .t-input 上调字号，避免在内部 wrap/inner 上重复加边
// 与 border-radius，造成视觉上"嵌套圆角容器"的错觉
:deep(.t-input),
:deep(.t-select),
:deep(.t-textarea),
:deep(.t-input-number) {
  width: 100%;
  font-size: 13px;
}

// 厂商选择器样式 — 移至非 scoped 块，因为 t-select popup 渲染到 body 下
// .provider-option 样式见文件末尾

// 复选框
:deep(.t-checkbox) {
  font-size: 13px;

  .t-checkbox__label {
    font-size: 13px;
    color: var(--td-text-color-primary);
  }
}

// API Key 输入：前置 lock 图标 + 后置可点击的"显示/隐藏"小眼睛。
// TDesign 默认会让 prefix-icon 显示成灰色，这里没动；suffix 上的眼睛
// 用 placeholder 色，hover 时切到主文本色，避免抢戏。
.api-key-input {
  :deep(.t-input__prefix) {
    color: var(--td-text-color-placeholder);
  }

  :deep(.t-input__suffix) {
    color: var(--td-text-color-placeholder);
  }

  .api-key-toggle {
    cursor: pointer;
    transition: color 0.15s ease;
    font-size: 16px;

    &:hover {
      color: var(--td-text-color-primary);
    }
  }
}

// API 测试区域 — 弱卡片化：用浅底 + dashed 边把"操作 + 反馈"框成一块，
// 让用户视觉上把它当成一个独立的"动作单元"，而不是又一个普通字段。
// （历史样式保留：仅当某个分支仍以 inline 方式渲染测试块时使用；当前 RemoteAPI
// 测试已上移到 SettingDrawer footer-left 槽，主流程不再走这块。）
.api-test-section {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  background: var(--td-bg-color-container-hover);
  border: 1px dashed var(--td-component-stroke);
  border-radius: 8px;

  .test-message {
    font-size: 13px;
    line-height: 1.5;
    flex: 1;

    &.success {
      color: var(--td-brand-color-active);
    }

    &.error {
      color: var(--td-error-color);
    }
  }

  :deep(.t-button) {
    min-width: 88px;
    height: 32px;
    font-size: 13px;
    border-radius: 6px;
    flex-shrink: 0;
  }

  .status-icon {
    font-size: 16px;
    flex-shrink: 0;

    &.available {
      color: var(--td-brand-color);
    }

    &.unavailable {
      color: var(--td-error-color);
    }
  }
}

// Connection-test message rendered next to the test button in the drawer
// footer. Truncates with ellipsis so a long backend error doesn't push
// Save/Cancel off-screen — the full text is in the title attribute.
.footer-test-message {
  font-size: 12px;
  line-height: 1.4;
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;

  &.success {
    color: var(--td-brand-color-active);
  }

  &.error {
    color: var(--td-error-color);
  }
}

// Status icon variant used inside the footer button.
.status-icon {
  font-size: 16px;
  flex-shrink: 0;

  &.available {
    color: var(--td-brand-color);
  }

  &.unavailable {
    color: var(--td-error-color);
  }
}

// 维度控制样式
.dimension-control {
  display: flex;
  align-items: center;
  gap: 8px;

  :deep(.t-input) {
    flex: 1;
  }
}

// 自定义 HTTP Header 区域
.custom-headers-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 6px;
}

.custom-headers-desc {
  margin: 0 0 10px 0;
  font-size: 12px;
  line-height: 1.5;
  color: var(--td-text-color-placeholder);
}

.custom-headers-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.custom-header-row {
  display: flex;
  align-items: center;
  gap: 8px;

  .custom-header-key {
    flex: 0 0 38%;
  }

  .custom-header-value {
    flex: 1;
  }

  // Ghost icon button — matches the model-card "more" affordance: invisible
  // until hover/focus, then a subtle background pops in. Avoids painting a
  // permanent red splotch next to every header row.
  .custom-header-remove {
    flex-shrink: 0;
    width: 32px;
    height: 32px;
    padding: 0;
    color: var(--td-text-color-placeholder);
    border-radius: 6px;
    transition: all 0.18s ease;

    &:hover {
      background: var(--td-error-color-light);
      color: var(--td-error-color);
    }
  }
}

.form-desc {
  margin: 4px 0 0 0;
  font-size: 12px;
  line-height: 1.5;
  color: var(--td-text-color-placeholder);

  // Inline with switches/checkboxes — drops the top margin so the label and
  // helper text sit on the same baseline.
  &--inline {
    margin: 0;
  }

  &--recommend {
    color: var(--td-brand-color);
  }

  &--warn {
    color: var(--td-warning-color);
  }
}

.vision-toggle {
  display: flex;
  align-items: center;
  gap: 8px;
}

// Destructive-action checkbox for "Remove this credential". Styled to match
// the pattern used in McpServiceDialog so the two dialogs read identically.
.clear-credential {
  display: inline-flex;
  margin-top: 8px;

  :deep(.t-checkbox__label) {
    color: var(--td-error-color);
    font-size: 13px;
  }
}
</style>

<!-- 非 scoped 样式：t-select popup 渲染到 body 下，scoped 样式无法覆盖 -->
<style lang="less">
.thinking-control-select-popup {
  min-width: 22rem;
  max-width: min(28rem, calc(100vw - 2rem));
  padding: 4px;

  .t-select-option {
    height: auto !important;
    padding: 8px 10px;
    border-radius: 6px;
    margin: 2px 0;
    white-space: normal;
  }
}

.thinking-control-option {
  display: flex;
  flex-direction: column;
  gap: 2px;
  line-height: 1.35;
  min-width: 0;

  &__title {
    font-size: 13px;
    color: var(--td-text-color-primary);
  }

  &__hint {
    font-size: 12px;
    color: var(--td-text-color-placeholder);
    word-break: break-word;
  }
}

.provider-select-popup {
  // 容器留点呼吸：避免选项贴着 popup 圆角
  padding: 4px;

  // TDesign 默认会在 t-select-option 上挂一个 overflow tooltip（浮在右侧
  // 显示完整 label）。我们的选项排版是「主名称 + 次描述」两行，永远不会
  // 触发省略，tooltip 反而成了视觉噪音 → 直接隐藏 popup 自带的提示。
  + .t-popup .t-tooltip,
  ~ .t-popup .t-tooltip {
    display: none !important;
  }

  .t-select-option {
    height: auto !important;
    padding: 8px 10px;
    border-radius: 6px;
    margin: 2px 0;
    outline: none;
    transition: background-color 0.15s ease;

    &:focus,
    &:focus-visible {
      outline: none;
    }

    // hover 态：用浅 brand 色而非强灰，跟主题色调一致
    &:hover:not(.t-is-selected) {
      background-color: var(--td-bg-color-container-hover);
    }
  }

  // 命中态：浅一点的底色 + 左侧主题色条作为 affordance，不再用全填的灰底
  .t-select-option.t-is-selected {
    background-color: var(--td-brand-color-light);
    color: var(--td-text-color-primary);
    font-weight: 500;
    position: relative;

    &::before {
      content: '';
      position: absolute;
      left: 0;
      top: 8px;
      bottom: 8px;
      width: 3px;
      background: var(--td-brand-color);
      border-radius: 0 2px 2px 0;
    }

    .provider-name {
      color: var(--td-brand-color);
    }
  }

  .provider-option {
    display: flex;
    flex-direction: column;
    gap: 2px;
    width: 100%;
    min-width: 0;

    .provider-name {
      font-size: 13px;
      font-weight: 500;
      color: var(--td-text-color-primary);
      line-height: 20px;
    }

    .provider-desc {
      font-size: 12px;
      color: var(--td-text-color-placeholder);
      line-height: 18px;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }
  }
}
</style>
