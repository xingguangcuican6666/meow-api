/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { t } from 'i18next'
import { z } from 'zod'

import { parseHttpStatusCodeRules } from '@/lib/http-status-code-rules'

import {
  BILLING_QUERY_TYPE_NEW_API,
  CLAUDE_FIELD_PASSTHROUGH_TYPES,
  CHANNEL_TYPE_NEW_API,
  CHANNEL_TYPE_OLLAMA,
  CHANNEL_TYPE_TASK_PLUGIN,
  CHANNEL_TYPE_VLLM,
  CHANNEL_TYPE_SGLANG,
  CHANNEL_STATUS,
  ERROR_MESSAGES,
  FIELD_PASSTHROUGH_TYPES,
  MODEL_FETCHABLE_TYPES,
  OPENAI_FIELD_PASSTHROUGH_TYPES,
  RATIO_PROBE_DEFAULT_PATH,
  RATIO_PROBE_MAX_AUTHORIZATION_LENGTH,
  RATIO_PROBE_MAX_RATIO,
  RATIO_PROBE_SOURCE_CUSTOM,
  RATIO_PROBE_SOURCE_FOLLOW_API,
} from '../constants'
import type { Channel } from '../types'
import {
  CHANNEL_TYPE_ADVANCED_CUSTOM,
  advancedCustomConfigUsesRelativeUpstreamPath,
  hasValidAdvancedCustomModelListRoute,
  parseAdvancedCustomConfig,
  stringifyAdvancedCustomConfig,
  validateAdvancedCustomConfig,
} from './advanced-custom'
import { readTaskExtendPluginKeys } from './channel-plugin-extensions'
import { validateModelMappingJson } from './model-mapping-validation'

// ============================================================================
// Form Validation Schema
// ============================================================================

const SUPPORTED_PROXY_PROTOCOLS = new Set([
  'http:',
  'https:',
  'socks5:',
  'socks5h:',
])

function isOptionalProxyURL(value: string | undefined): boolean {
  const trimmedValue = value?.trim() || ''
  if (!trimmedValue) return true

  const schemeSeparatorIndex = trimmedValue.indexOf('://')
  if (schemeSeparatorIndex <= 0) return false

  const authorityAndSuffix = trimmedValue.slice(schemeSeparatorIndex + 3)
  const suffixIndex = authorityAndSuffix.search(/[/?#]/)
  if (suffixIndex >= 0 && authorityAndSuffix.slice(suffixIndex) !== '/') {
    return false
  }

  try {
    const parsedURL = new URL(trimmedValue)
    return (
      SUPPORTED_PROXY_PROTOCOLS.has(parsedURL.protocol) &&
      Boolean(parsedURL.hostname) &&
      parsedURL.port !== '0'
    )
  } catch {
    return false
  }
}

export const HTTP_PROTOCOL_AUTO = 'auto'
export const HTTP_PROTOCOL_HTTP1 = 'http1'
export const MAX_HTTP2_CONNECTION_SHARDS = 8

export function normalizeHttpProtocol(
  value: string | undefined | null
): 'auto' | 'http1' {
  const normalized = String(value || '')
    .trim()
    .toLowerCase()
  if (normalized === HTTP_PROTOCOL_HTTP1) {
    return HTTP_PROTOCOL_HTTP1
  }
  return HTTP_PROTOCOL_AUTO
}

export function normalizeHttp2ConnectionShards(
  value: number | undefined | null
): number {
  if (value == null || Number.isNaN(value) || value === 0) {
    return 1
  }
  if (value < 1) {
    return 1
  }
  if (value > MAX_HTTP2_CONNECTION_SHARDS) {
    return MAX_HTTP2_CONNECTION_SHARDS
  }
  return value
}

function parseOptionalJson(value: string | undefined): unknown {
  if (!value?.trim()) return undefined
  return JSON.parse(value)
}

function isJsonObjectValue(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function isOptionalJsonObject(value: string | undefined): boolean {
  try {
    const parsed = parseOptionalJson(value)
    return parsed === undefined || isJsonObjectValue(parsed)
  } catch {
    return false
  }
}

function isOptionalModelMapping(value: string | undefined): boolean {
  try {
    if (parseOptionalJson(value) === undefined) return true
  } catch {
    return false
  }
  return validateModelMappingJson(value ?? '').valid
}

function isOptionalStatusCodeMapping(value: string | undefined): boolean {
  try {
    const parsed = parseOptionalJson(value)
    if (parsed === undefined) return true
    if (!isJsonObjectValue(parsed)) return false
    return Object.entries(parsed).every(([from, to]) => {
      const fromCode = Number(from)
      const toCode = Number(to)
      return (
        Number.isInteger(fromCode) &&
        Number.isInteger(toCode) &&
        fromCode >= 100 &&
        fromCode <= 599 &&
        toCode >= 100 &&
        toCode <= 599
      )
    })
  } catch {
    return false
  }
}

function isCodexCredential(value: string | undefined): boolean {
  try {
    const parsed = parseOptionalJson(value)
    if (parsed === undefined) return true
    return (
      isJsonObjectValue(parsed) &&
      typeof parsed.access_token === 'string' &&
      parsed.access_token.trim().length > 0 &&
      typeof parsed.account_id === 'string' &&
      parsed.account_id.trim().length > 0
    )
  } catch {
    return false
  }
}

function isVertexJsonKey(value: string | undefined): boolean {
  try {
    const parsed = parseOptionalJson(value)
    if (parsed === undefined) return true
    if (Array.isArray(parsed)) {
      return parsed.every((item) => isJsonObjectValue(item))
    }
    return isJsonObjectValue(parsed)
  } catch {
    return false
  }
}

// Shared by the balance query and the upstream ratio probe: both post to an
// independent New API instance and both reject embedded credentials so a saved
// channel setting cannot smuggle secrets into a URL.
function isValidUpstreamBaseURL(value: string | undefined): boolean {
  const trimmedValue = value?.trim() || ''
  if (!trimmedValue) return false

  try {
    const parsedURL = new URL(trimmedValue)
    return (
      (parsedURL.protocol === 'http:' || parsedURL.protocol === 'https:') &&
      Boolean(parsedURL.hostname) &&
      !parsedURL.username &&
      !parsedURL.password &&
      !parsedURL.search &&
      !parsedURL.hash &&
      !trimmedValue.includes('?') &&
      !trimmedValue.includes('#')
    )
  } catch {
    return false
  }
}

const billingQuerySchema = z.object({
  type: z.enum(['', BILLING_QUERY_TYPE_NEW_API]).optional(),
  base_url: z.string().optional(),
  bearer_token: z.string().optional(),
  user_id: z.string().optional(),
  use_api_key: z.boolean().optional(),
})

export const IMAGE_UPSCALE_ON_ERROR_FALLBACK = 'fallback'
export const IMAGE_UPSCALE_ON_ERROR_FAIL = 'fail'
export const IMAGE_UPSCALE_DEFAULT_TIMEOUT_SECONDS = 300
export const IMAGE_UPSCALE_MAX_TIMEOUT_SECONDS = 3600

const imageUpscaleSchema = z.object({
  enabled: z.boolean().optional(),
  target_channel_id: z.number().int().optional(),
  target_model: z.string().optional(),
  // Comma-separated source model allowlist; stored as an array in settings JSON.
  models: z.string().optional(),
  on_error: z
    .enum(['', IMAGE_UPSCALE_ON_ERROR_FALLBACK, IMAGE_UPSCALE_ON_ERROR_FAIL])
    .optional(),
  timeout_seconds: z.number().int().optional(),
})

const ratioProbeSchema = z.object({
  enabled: z.boolean().optional(),
  source: z
    .enum([RATIO_PROBE_SOURCE_FOLLOW_API, RATIO_PROBE_SOURCE_CUSTOM])
    .optional(),
  base_url: z.string().optional(),
  path: z.string().optional(),
  group: z.string().optional(),
  // The bounds stay strings so an empty field means "no bound" instead of 0.
  max_group_ratio: z.string().optional(),
  min_group_ratio: z.string().optional(),
  use_api_key: z.boolean().optional(),
  authorization: z.string().optional(),
})

export type RatioProbeBound = number | undefined

/**
 * Parse one ratio bound. Returns undefined for an empty field (no bound) and
 * null when the text is not a usable ratio.
 */
export function parseRatioProbeBound(
  value: string | undefined
): RatioProbeBound | null {
  const trimmedValue = String(value ?? '').trim()
  if (!trimmedValue) return undefined
  const parsedValue = Number(trimmedValue)
  if (!Number.isFinite(parsedValue)) return null
  if (parsedValue < 0 || parsedValue > RATIO_PROBE_MAX_RATIO) return null
  return parsedValue
}

function isValidRatioProbePath(value: string | undefined): boolean {
  const trimmedValue = String(value ?? '').trim()
  if (!trimmedValue) return true
  if (!trimmedValue.startsWith('/') || trimmedValue.startsWith('//')) {
    return false
  }
  return !trimmedValue.includes('?') && !trimmedValue.includes('#')
}

/**
 * Mirrors relaykit/dto.validateRatioProbeAuthorization: the stored value must be
 * usable as an HTTP header, so control characters that could inject a second
 * header are rejected here as well.
 */
function isValidRatioProbeAuthorization(value: string): boolean {
  for (const character of value) {
    const code = character.codePointAt(0) ?? 0
    if (code !== 9 && (code < 0x20 || code > 0x7e)) return false
  }
  return true
}

function addRequiredIssue(
  ctx: z.RefinementCtx,
  path: string,
  message: string
): void {
  ctx.addIssue({
    code: z.ZodIssueCode.custom,
    path: [path],
    message,
  })
}

function addRatioProbeIssue(
  ctx: z.RefinementCtx,
  field: string,
  message: string
): void {
  ctx.addIssue({
    code: z.ZodIssueCode.custom,
    path: ['ratio_probe', field],
    message,
  })
}

/**
 * Mirrors the backend contract in relaykit/dto.RatioProbeConfig.Validate so an
 * unusable probe is rejected before it reaches the scheduled task.
 */
function validateRatioProbeForm(
  probe: z.infer<typeof ratioProbeSchema>,
  ctx: z.RefinementCtx
): void {
  if (probe.source === RATIO_PROBE_SOURCE_CUSTOM) {
    if (!probe.base_url?.trim()) {
      addRatioProbeIssue(ctx, 'base_url', 'Ratio probe Base URL is required')
    } else if (!isValidUpstreamBaseURL(probe.base_url)) {
      addRatioProbeIssue(
        ctx,
        'base_url',
        'Ratio probe Base URL must be an absolute HTTP or HTTPS URL without credentials, query parameters, or fragments'
      )
    }
  }

  if (!isValidRatioProbePath(probe.path)) {
    addRatioProbeIssue(
      ctx,
      'path',
      'Ratio probe path must be an absolute path without query parameters or fragments'
    )
  }

  const authorization = probe.authorization?.trim() || ''
  if (authorization.length > RATIO_PROBE_MAX_AUTHORIZATION_LENGTH) {
    addRatioProbeIssue(ctx, 'authorization', 'Authorization header is too long')
  } else if (!isValidRatioProbeAuthorization(authorization)) {
    addRatioProbeIssue(
      ctx,
      'authorization',
      'Authorization header must use printable ASCII characters without line breaks'
    )
  }

  const maxRatio = parseRatioProbeBound(probe.max_group_ratio)
  const minRatio = parseRatioProbeBound(probe.min_group_ratio)
  if (maxRatio === null) {
    addRatioProbeIssue(ctx, 'max_group_ratio', 'Ratio bound is out of range')
  }
  if (minRatio === null) {
    addRatioProbeIssue(ctx, 'min_group_ratio', 'Ratio bound is out of range')
  }
  if (maxRatio === undefined && minRatio === undefined) {
    addRatioProbeIssue(
      ctx,
      'max_group_ratio',
      'Set a max or min group ratio to judge the upstream'
    )
  }
  if (
    typeof maxRatio === 'number' &&
    typeof minRatio === 'number' &&
    minRatio > maxRatio
  ) {
    addRatioProbeIssue(
      ctx,
      'min_group_ratio',
      'Min group ratio must not exceed max group ratio'
    )
  }
}

export const channelFormSchema = z
  .object({
    name: z.string().min(1, ERROR_MESSAGES.REQUIRED_NAME),
    type: z.number().min(0, ERROR_MESSAGES.REQUIRED_TYPE),
    base_url: z.string().optional(),
    task_plugin_key: z.string().optional(),
    task_extend_plugin_keys: z.array(z.string()).optional(),
    key: z.string(),
    openai_organization: z.string().optional(),
    models: z.string().min(1, ERROR_MESSAGES.REQUIRED_MODELS),
    group: z.array(z.string()).min(1, ERROR_MESSAGES.REQUIRED_GROUP),
    model_mapping: z
      .string()
      .optional()
      .refine(
        isOptionalModelMapping,
        'Model mapping must be a JSON object of strings or ordered model queues'
      ),
    priority: z.number().optional(),
    weight: z.number().optional(),
    test_model: z.string().optional(),
    auto_ban: z.number().optional(),
    runtime_automatic_disable_override_enabled: z.boolean().optional(),
    runtime_automatic_disable_status_codes: z.string().optional(),
    runtime_automatic_disable_keywords: z.string().optional(),
    empty_response_retry_override_enabled: z.boolean().optional(),
    empty_response_retry_enabled: z.boolean().optional(),
    empty_response_retry_in_place: z.boolean().optional(),
    automatic_retry_override_enabled: z.boolean().optional(),
    automatic_retry_status_codes: z.string().optional(),
    automatic_retry_keywords: z.string().optional(),
    status: z.number(),
    status_code_mapping: z
      .string()
      .optional()
      .refine(
        isOptionalStatusCodeMapping,
        'Status code mapping must use valid HTTP status codes'
      ),
    tag: z.string().optional(),
    remark: z
      .string()
      .max(255, 'Remark must be less than 255 characters')
      .optional(),
    setting: z
      .string()
      .optional()
      .refine(isOptionalJsonObject, ERROR_MESSAGES.INVALID_JSON),
    param_override: z
      .string()
      .optional()
      .refine(isOptionalJsonObject, ERROR_MESSAGES.INVALID_JSON),
    header_override: z
      .string()
      .optional()
      .refine(isOptionalJsonObject, ERROR_MESSAGES.INVALID_JSON),
    settings: z
      .string()
      .optional()
      .refine(isOptionalJsonObject, ERROR_MESSAGES.INVALID_JSON),
    advanced_custom: z.string().optional(),
    billing_query: billingQuerySchema.optional(),
    ratio_probe: ratioProbeSchema.optional(),
    image_upscale: imageUpscaleSchema.optional(),
    other: z.string().optional(),
    // Multi-key options (not sent to backend directly)
    multi_key_mode: z.enum(['single', 'batch', 'multi_to_single']).optional(),
    multi_key_type: z.enum(['random', 'polling']).optional(),
    batch_add_set_key_prefix_2_name: z.boolean().optional(),
    key_mode: z.enum(['append', 'replace']).optional(), // For editing multi-key channels
    // Channel extra settings (stored in setting JSON, not sent directly)
    force_format: z.boolean().optional(),
    thinking_to_content: z.boolean().optional(),
    proxy: z
      .string()
      .optional()
      .refine(isOptionalProxyURL, ERROR_MESSAGES.INVALID_PROXY),
    http_protocol: z.enum(['auto', 'http1']).optional(),
    http2_connection_shards: z.number().int().optional(),
    pass_through_body_enabled: z.boolean().optional(),
    responses_websocket_enabled: z.boolean().optional(),
    system_prompt: z.string().optional(),
    system_prompt_override: z.boolean().optional(),
    // Type-specific settings (stored in settings JSON)
    is_enterprise_account: z.boolean().optional(), // OpenRouter specific
    vertex_key_type: z.enum(['json', 'api_key']).optional(), // Vertex AI specific
    aws_key_type: z.enum(['ak_sk', 'api_key']).optional(), // AWS specific
    azure_responses_version: z.string().optional(), // Azure specific
    // Field passthrough controls (stored in settings JSON)
    allow_service_tier: z.boolean().optional(), // OpenAI/Anthropic
    disable_store: z.boolean().optional(), // OpenAI only
    allow_safety_identifier: z.boolean().optional(), // OpenAI only
    allow_include_obfuscation: z.boolean().optional(), // OpenAI: include usage obfuscation
    allow_inference_geo: z.boolean().optional(), // OpenAI/Anthropic: inference geography
    allow_speed: z.boolean().optional(), // Anthropic: speed mode control
    claude_beta_query: z.boolean().optional(), // Anthropic: beta query passthrough
    ollama_openai_chat: z.boolean().optional(), // Ollama: OpenAI-compatible /v1/chat/completions instead of native /api/chat
    disable_task_polling_sleep: z.boolean().optional(),
    // Upstream model update settings (stored in settings JSON)
    upstream_model_update_check_enabled: z.boolean().optional(),
    upstream_model_update_auto_sync_enabled: z.boolean().optional(),
    upstream_model_update_ignored_models: z.string().optional(),
  })
  .superRefine((data, ctx) => {
    const billingQuery = data.billing_query
    if (billingQuery?.type === BILLING_QUERY_TYPE_NEW_API) {
      if (!billingQuery.base_url?.trim()) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['billing_query', 'base_url'],
          message: 'Balance query Base URL is required',
        })
      } else if (!isValidUpstreamBaseURL(billingQuery.base_url)) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['billing_query', 'base_url'],
          message:
            'Balance query Base URL must be an absolute HTTP or HTTPS URL without credentials, query parameters, or fragments',
        })
      }
    }

    if (data.ratio_probe?.enabled) {
      validateRatioProbeForm(data.ratio_probe, ctx)
    }

    if (data.image_upscale?.enabled) {
      const upscale = data.image_upscale
      if (
        !upscale.target_channel_id ||
        upscale.target_channel_id < 1 ||
        !Number.isInteger(upscale.target_channel_id)
      ) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['image_upscale', 'target_channel_id'],
          message: 'Image upscale target channel ID must be a positive integer',
        })
      }
      if (!upscale.target_model?.trim()) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['image_upscale', 'target_model'],
          message: 'Image upscale target model is required',
        })
      }
      const timeout = upscale.timeout_seconds ?? 0
      if (timeout < 0 || timeout > IMAGE_UPSCALE_MAX_TIMEOUT_SECONDS) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['image_upscale', 'timeout_seconds'],
          message: 'Image upscale timeout must be between 0 and 3600 seconds',
        })
      }
    }

    if (data.runtime_automatic_disable_override_enabled) {
      const parsed = parseHttpStatusCodeRules(
        data.runtime_automatic_disable_status_codes
      )
      if (!parsed.ok) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['runtime_automatic_disable_status_codes'],
          message: t('Automatic disable status codes must be valid HTTP codes'),
        })
      }
    }

    if (data.automatic_retry_override_enabled) {
      const parsed = parseHttpStatusCodeRules(data.automatic_retry_status_codes)
      if (!parsed.ok) {
        ctx.addIssue({
          code: z.ZodIssueCode.custom,
          path: ['automatic_retry_status_codes'],
          message: t('Forced retry status codes must be valid HTTP codes'),
        })
      }
    }
  })
  .superRefine((data, ctx) => {
    if (
      [
        3,
        8,
        36,
        45,
        CHANNEL_TYPE_NEW_API,
        CHANNEL_TYPE_TASK_PLUGIN,
        CHANNEL_TYPE_VLLM,
        CHANNEL_TYPE_SGLANG,
      ].includes(data.type) &&
      !data.base_url?.trim()
    ) {
      addRequiredIssue(
        ctx,
        'base_url',
        'Base URL is required for this channel type'
      )
    }
    if (
      data.type === CHANNEL_TYPE_TASK_PLUGIN &&
      !data.task_plugin_key?.trim()
    ) {
      addRequiredIssue(ctx, 'task_plugin_key', 'Task plugin is required')
    }

    if (data.type === CHANNEL_TYPE_ADVANCED_CUSTOM) {
      const advancedCustomConfig = parseAdvancedCustomConfig(
        data.advanced_custom
      )
      const advancedCustomError =
        validateAdvancedCustomConfig(advancedCustomConfig)
      if (advancedCustomError) {
        addRequiredIssue(ctx, 'advanced_custom', advancedCustomError.message)
      }
      if (
        advancedCustomConfigUsesRelativeUpstreamPath(advancedCustomConfig) &&
        !data.base_url?.trim()
      ) {
        addRequiredIssue(
          ctx,
          'base_url',
          'Base URL is required when an advanced route uses an upstream path'
        )
      }
      if (
        data.upstream_model_update_check_enabled === true &&
        !hasValidAdvancedCustomModelListRoute(advancedCustomConfig)
      ) {
        addRequiredIssue(
          ctx,
          'upstream_model_update_check_enabled',
          'OpenAI Models route is required to enable upstream model checks'
        )
      }
    }

    if ([3, 18, 21, 39, 41, 49].includes(data.type) && !data.other?.trim()) {
      addRequiredIssue(
        ctx,
        'other',
        'This channel type requires additional configuration'
      )
    }

    if (data.type === 57) {
      if (data.multi_key_mode && data.multi_key_mode !== 'single') {
        addRequiredIssue(
          ctx,
          'multi_key_mode',
          'Codex channels do not support batch creation'
        )
      }
      if (data.key?.trim() && !isCodexCredential(data.key)) {
        addRequiredIssue(
          ctx,
          'key',
          'Codex credential must be a JSON object with access_token and account_id'
        )
      }
    }

    if (
      data.type === 41 &&
      data.vertex_key_type === 'json' &&
      data.key?.trim() &&
      !isVertexJsonKey(data.key)
    ) {
      addRequiredIssue(
        ctx,
        'key',
        'Vertex AI service account key must be valid JSON'
      )
    }

    if (
      data.type === 41 &&
      data.vertex_key_type === 'api_key' &&
      data.multi_key_mode &&
      data.multi_key_mode !== 'single'
    ) {
      addRequiredIssue(
        ctx,
        'multi_key_mode',
        'Vertex AI API Key mode does not support batch creation'
      )
    }

    const protocol = normalizeHttpProtocol(data.http_protocol)
    const shards = data.http2_connection_shards ?? 1
    if (shards < 1 || shards > MAX_HTTP2_CONNECTION_SHARDS) {
      addRequiredIssue(
        ctx,
        'http2_connection_shards',
        ERROR_MESSAGES.INVALID_HTTP2_CONNECTION_SHARDS
      )
    }
    if (protocol === HTTP_PROTOCOL_HTTP1 && shards > 1) {
      addRequiredIssue(
        ctx,
        'http2_connection_shards',
        ERROR_MESSAGES.INVALID_HTTP1_WITH_SHARDS
      )
    }
  })

export type ChannelFormValues = z.infer<typeof channelFormSchema>

// ============================================================================
// Default Form Values
// ============================================================================

export const CHANNEL_FORM_DEFAULT_VALUES: ChannelFormValues = {
  name: '',
  type: 1,
  base_url: '',
  task_plugin_key: '',
  task_extend_plugin_keys: [],
  key: '',
  openai_organization: '',
  models: '',
  group: ['default'],
  model_mapping: '',
  priority: 0,
  weight: 0,
  test_model: '',
  auto_ban: 1,
  runtime_automatic_disable_override_enabled: false,
  runtime_automatic_disable_status_codes: '',
  runtime_automatic_disable_keywords: '',
  empty_response_retry_override_enabled: false,
  empty_response_retry_enabled: false,
  empty_response_retry_in_place: true,
  automatic_retry_override_enabled: false,
  automatic_retry_status_codes: '',
  automatic_retry_keywords: '',
  status: CHANNEL_STATUS.ENABLED,
  status_code_mapping: '',
  tag: '',
  remark: '',
  setting: '',
  param_override: '',
  header_override: '',
  settings: '{}',
  other: '',
  multi_key_mode: 'single',
  multi_key_type: 'random',
  batch_add_set_key_prefix_2_name: false,
  key_mode: 'append',
  // Channel extra settings
  force_format: false,
  thinking_to_content: false,
  proxy: '',
  http_protocol: HTTP_PROTOCOL_AUTO,
  http2_connection_shards: 1,
  pass_through_body_enabled: false,
  responses_websocket_enabled: false,
  system_prompt: '',
  system_prompt_override: false,
  // Type-specific settings
  is_enterprise_account: false,
  vertex_key_type: 'json',
  aws_key_type: 'ak_sk',
  azure_responses_version: '',
  // Field passthrough controls
  allow_service_tier: false,
  disable_store: false,
  allow_safety_identifier: false,
  allow_include_obfuscation: false,
  allow_inference_geo: false,
  allow_speed: false,
  claude_beta_query: false,
  ollama_openai_chat: false,
  disable_task_polling_sleep: false,
  upstream_model_update_check_enabled: false,
  upstream_model_update_auto_sync_enabled: false,
  upstream_model_update_ignored_models: '',
  advanced_custom: '',
  billing_query: {
    type: '',
    base_url: '',
    bearer_token: '',
    user_id: '',
    use_api_key: true,
  },
  ratio_probe: {
    enabled: false,
    source: RATIO_PROBE_SOURCE_FOLLOW_API,
    base_url: '',
    path: '',
    group: '',
    max_group_ratio: '',
    min_group_ratio: '',
    use_api_key: true,
    authorization: '',
  },
  image_upscale: {
    enabled: false,
    target_channel_id: 0,
    target_model: '',
    models: '',
    on_error: IMAGE_UPSCALE_ON_ERROR_FALLBACK,
    timeout_seconds: IMAGE_UPSCALE_DEFAULT_TIMEOUT_SECONDS,
  },
}

type RatioProbeFormValues = {
  enabled: boolean
  source:
    | typeof RATIO_PROBE_SOURCE_FOLLOW_API
    | typeof RATIO_PROBE_SOURCE_CUSTOM
  base_url: string
  path: string
  group: string
  max_group_ratio: string
  min_group_ratio: string
  use_api_key: boolean
  authorization: string
}

const DEFAULT_RATIO_PROBE_FORM_VALUES: RatioProbeFormValues = {
  enabled: false,
  source: RATIO_PROBE_SOURCE_FOLLOW_API,
  base_url: '',
  path: '',
  group: '',
  max_group_ratio: '',
  min_group_ratio: '',
  use_api_key: true,
  authorization: '',
}

function ratioProbeBoundToInput(value: unknown): string {
  return typeof value === 'number' && Number.isFinite(value)
    ? String(value)
    : ''
}

function normalizeRatioProbeFormValues(value: unknown): RatioProbeFormValues {
  if (!isJsonObjectValue(value)) {
    return { ...DEFAULT_RATIO_PROBE_FORM_VALUES }
  }

  return {
    enabled: value.enabled === true,
    source:
      value.source === RATIO_PROBE_SOURCE_CUSTOM
        ? RATIO_PROBE_SOURCE_CUSTOM
        : RATIO_PROBE_SOURCE_FOLLOW_API,
    base_url: typeof value.base_url === 'string' ? value.base_url : '',
    path: typeof value.path === 'string' ? value.path : '',
    group: typeof value.group === 'string' ? value.group : '',
    max_group_ratio: ratioProbeBoundToInput(value.max_group_ratio),
    min_group_ratio: ratioProbeBoundToInput(value.min_group_ratio),
    use_api_key: value.use_api_key !== false,
    authorization:
      typeof value.authorization === 'string' ? value.authorization : '',
  }
}

type BillingQueryFormValues = {
  type: '' | typeof BILLING_QUERY_TYPE_NEW_API
  base_url: string
  bearer_token: string
  user_id: string
  use_api_key: boolean
}

const DEFAULT_BILLING_QUERY_FORM_VALUES: BillingQueryFormValues = {
  type: '',
  base_url: '',
  bearer_token: '',
  user_id: '',
  use_api_key: true,
}

function normalizeBillingQueryFormValues(
  value: unknown
): BillingQueryFormValues {
  if (!isJsonObjectValue(value)) {
    return { ...DEFAULT_BILLING_QUERY_FORM_VALUES }
  }

  return {
    type:
      value.type === BILLING_QUERY_TYPE_NEW_API
        ? BILLING_QUERY_TYPE_NEW_API
        : '',
    base_url: typeof value.base_url === 'string' ? value.base_url : '',
    bearer_token:
      typeof value.bearer_token === 'string' ? value.bearer_token : '',
    user_id: typeof value.user_id === 'string' ? value.user_id : '',
    use_api_key: value.use_api_key !== false,
  }
}

type ImageUpscaleFormValues = z.infer<typeof imageUpscaleSchema>

const DEFAULT_IMAGE_UPSCALE_FORM_VALUES: ImageUpscaleFormValues = {
  enabled: false,
  target_channel_id: 0,
  target_model: '',
  models: '',
  on_error: IMAGE_UPSCALE_ON_ERROR_FALLBACK,
  timeout_seconds: IMAGE_UPSCALE_DEFAULT_TIMEOUT_SECONDS,
}

function normalizeImageUpscaleFormValues(value: unknown): ImageUpscaleFormValues {
  if (!isJsonObjectValue(value)) {
    return { ...DEFAULT_IMAGE_UPSCALE_FORM_VALUES }
  }

  const onError =
    value.on_error === IMAGE_UPSCALE_ON_ERROR_FAIL
      ? IMAGE_UPSCALE_ON_ERROR_FAIL
      : IMAGE_UPSCALE_ON_ERROR_FALLBACK
  const targetChannelId =
    typeof value.target_channel_id === 'number' &&
    Number.isFinite(value.target_channel_id)
      ? Math.trunc(value.target_channel_id)
      : 0
  const timeoutSeconds =
    typeof value.timeout_seconds === 'number' &&
    Number.isFinite(value.timeout_seconds)
      ? Math.trunc(value.timeout_seconds)
      : IMAGE_UPSCALE_DEFAULT_TIMEOUT_SECONDS
  const models = Array.isArray(value.models)
    ? value.models.filter((model): model is string => typeof model === 'string')
    : []

  return {
    enabled: value.enabled === true,
    target_channel_id: targetChannelId,
    target_model: typeof value.target_model === 'string' ? value.target_model : '',
    models: models.join(','),
    on_error: onError,
    timeout_seconds: timeoutSeconds,
  }
}

// ============================================================================
// Transform Functions
// ============================================================================

/**
 * Transform Channel from API to Form default values
 */
export function transformChannelToFormDefaults(
  channel: Channel
): ChannelFormValues {
  // Parse channel extra settings from setting field
  let extraSettings = {
    task_plugin_key: '',
    task_extend_plugin_keys: [] as string[],
    force_format: false,
    thinking_to_content: false,
    proxy: '',
    http_protocol: HTTP_PROTOCOL_AUTO as 'auto' | 'http1',
    http2_connection_shards: 1,
    pass_through_body_enabled: false,
    responses_websocket_enabled: false,
    system_prompt: '',
    system_prompt_override: false,
  }

  if (channel.setting) {
    try {
      const parsed = JSON.parse(channel.setting)
      const protocol = normalizeHttpProtocol(parsed.http_protocol)
      const shards = normalizeHttp2ConnectionShards(
        parsed.http2_connection_shards
      )
      extraSettings = {
        task_plugin_key: parsed.task_plugin_key || '',
        task_extend_plugin_keys: readTaskExtendPluginKeys(channel.type, parsed),
        force_format: parsed.force_format || false,
        thinking_to_content: parsed.thinking_to_content || false,
        proxy: parsed.proxy || '',
        http_protocol: protocol,
        http2_connection_shards: protocol === HTTP_PROTOCOL_HTTP1 ? 1 : shards,
        pass_through_body_enabled: parsed.pass_through_body_enabled || false,
        responses_websocket_enabled:
          parsed.responses_websocket_enabled === true,
        system_prompt: parsed.system_prompt || '',
        system_prompt_override: parsed.system_prompt_override || false,
      }
    } catch (error) {
      // eslint-disable-next-line no-console
      console.error('Failed to parse channel setting:', error)
    }
  }

  // Parse type-specific settings from settings field
  let vertexKeyType: 'json' | 'api_key' = 'json'
  let azureResponsesVersion = ''
  let isEnterpriseAccount = false
  let awsKeyType: 'ak_sk' | 'api_key' = 'ak_sk'
  let allowServiceTier = false
  let disableStore = false
  let allowSafetyIdentifier = false
  let allowIncludeObfuscation = false
  let allowInferenceGeo = false
  let allowSpeed = false
  let claudeBetaQuery = false
  let ollamaOpenAIChat = false
  let disableTaskPollingSleep = false
  let upstreamModelUpdateCheckEnabled = false
  let upstreamModelUpdateAutoSyncEnabled = false
  let upstreamModelUpdateIgnoredModels = ''
  let advancedCustom = ''
  let automaticDisableOverrideEnabled = false
  let automaticDisableStatusCodes = ''
  let automaticDisableKeywords = ''
  let emptyResponseRetryOverrideEnabled = false
  let emptyResponseRetryEnabled = false
  let emptyResponseRetryInPlace = true
  let automaticRetryOverrideEnabled = false
  let automaticRetryStatusCodes = ''
  let automaticRetryKeywords = ''
  let billingQuery = { ...DEFAULT_BILLING_QUERY_FORM_VALUES }
  let ratioProbe = { ...DEFAULT_RATIO_PROBE_FORM_VALUES }
  let imageUpscale = { ...DEFAULT_IMAGE_UPSCALE_FORM_VALUES }

  if (channel.settings) {
    try {
      const parsed = JSON.parse(channel.settings)
      vertexKeyType = parsed.vertex_key_type || 'json'
      azureResponsesVersion = parsed.azure_responses_version || ''
      isEnterpriseAccount = parsed.openrouter_enterprise === true
      awsKeyType = parsed.aws_key_type || 'ak_sk'
      allowServiceTier = parsed.allow_service_tier === true
      disableStore = parsed.disable_store === true
      allowSafetyIdentifier = parsed.allow_safety_identifier === true
      allowIncludeObfuscation = parsed.allow_include_obfuscation === true
      allowInferenceGeo = parsed.allow_inference_geo === true
      allowSpeed = parsed.allow_speed === true
      claudeBetaQuery = parsed.claude_beta_query === true
      ollamaOpenAIChat = parsed.ollama_openai_chat === true
      disableTaskPollingSleep = parsed.disable_task_polling_sleep === true
      upstreamModelUpdateCheckEnabled =
        parsed.upstream_model_update_check_enabled === true
      upstreamModelUpdateAutoSyncEnabled =
        parsed.upstream_model_update_auto_sync_enabled === true
      upstreamModelUpdateIgnoredModels = Array.isArray(
        parsed.upstream_model_update_ignored_models
      )
        ? parsed.upstream_model_update_ignored_models.join(',')
        : ''
      if (parsed.advanced_custom) {
        advancedCustom = stringifyAdvancedCustomConfig(parsed.advanced_custom)
      }
      billingQuery = normalizeBillingQueryFormValues(parsed.billing_query)
      ratioProbe = normalizeRatioProbeFormValues(parsed.ratio_probe)
      imageUpscale = normalizeImageUpscaleFormValues(parsed.image_upscale)
      automaticDisableOverrideEnabled =
        parsed.runtime_automatic_disable_override_enabled === true ||
        parsed.automatic_disable_override_enabled === true
      automaticDisableStatusCodes =
        parsed.runtime_automatic_disable_status_codes ||
        parsed.automatic_disable_status_codes ||
        ''
      automaticDisableKeywords =
        parsed.runtime_automatic_disable_keywords ||
        parsed.automatic_disable_keywords ||
        ''
      emptyResponseRetryOverrideEnabled =
        parsed.empty_response_retry_override_enabled === true
      emptyResponseRetryEnabled = parsed.empty_response_retry_enabled === true
      emptyResponseRetryInPlace = parsed.empty_response_retry_in_place === true
      automaticRetryOverrideEnabled =
        parsed.automatic_retry_override_enabled === true
      automaticRetryStatusCodes = parsed.automatic_retry_status_codes || ''
      automaticRetryKeywords = parsed.automatic_retry_keywords || ''
    } catch (error) {
      // eslint-disable-next-line no-console
      console.error('Failed to parse channel settings:', error)
    }
  }

  return {
    name: channel.name || '',
    type: channel.type,
    base_url: channel.base_url || '',
    key: '', // Never populate key from backend for security
    openai_organization: channel.openai_organization || '',
    models: channel.models || '',
    group: parseGroups(channel.group || 'default'),
    model_mapping: channel.model_mapping || '',
    priority: channel.priority || 0,
    weight: channel.weight || 0,
    test_model: channel.test_model || '',
    auto_ban: channel.auto_ban ?? 1,
    status: channel.status,
    status_code_mapping: channel.status_code_mapping || '',
    tag: channel.tag || '',
    remark: channel.remark || '',
    setting: channel.setting || '',
    param_override: channel.param_override || '',
    header_override: channel.header_override || '',
    settings: channel.settings || '{}',
    other: channel.other || '',
    multi_key_mode: 'single',
    multi_key_type: channel.channel_info.multi_key_mode || 'random',
    batch_add_set_key_prefix_2_name: false,
    key_mode: 'append', // Default to append mode for editing multi-key channels
    // Channel extra settings
    ...extraSettings,
    // Type-specific settings
    is_enterprise_account: isEnterpriseAccount,
    vertex_key_type: vertexKeyType,
    azure_responses_version: azureResponsesVersion,
    aws_key_type: awsKeyType,
    allow_service_tier: allowServiceTier,
    disable_store: disableStore,
    allow_include_obfuscation: allowIncludeObfuscation,
    allow_inference_geo: allowInferenceGeo,
    allow_speed: allowSpeed,
    claude_beta_query: claudeBetaQuery,
    ollama_openai_chat: ollamaOpenAIChat,
    disable_task_polling_sleep: disableTaskPollingSleep,
    allow_safety_identifier: allowSafetyIdentifier,
    upstream_model_update_check_enabled: upstreamModelUpdateCheckEnabled,
    upstream_model_update_auto_sync_enabled: upstreamModelUpdateAutoSyncEnabled,
    upstream_model_update_ignored_models: upstreamModelUpdateIgnoredModels,
    advanced_custom: advancedCustom,
    billing_query: billingQuery,
    ratio_probe: ratioProbe,
    image_upscale: imageUpscale,
    runtime_automatic_disable_override_enabled: automaticDisableOverrideEnabled,
    runtime_automatic_disable_status_codes: automaticDisableStatusCodes,
    runtime_automatic_disable_keywords: automaticDisableKeywords,
    empty_response_retry_override_enabled: emptyResponseRetryOverrideEnabled,
    empty_response_retry_enabled: emptyResponseRetryEnabled,
    empty_response_retry_in_place: emptyResponseRetryInPlace,
    automatic_retry_override_enabled: automaticRetryOverrideEnabled,
    automatic_retry_status_codes: automaticRetryStatusCodes,
    automatic_retry_keywords: automaticRetryKeywords,
  }
}

/**
 * Build the setting JSON string from form extra settings
 */
export function buildSettingJSON(formData: ChannelFormValues): string {
  const settingObj: Record<string, unknown> = {
    task_plugin_key:
      formData.type === CHANNEL_TYPE_TASK_PLUGIN
        ? formData.task_plugin_key?.trim() || ''
        : undefined,
    task_extend_plugin_keys:
      formData.type === CHANNEL_TYPE_NEW_API &&
      formData.task_extend_plugin_keys?.length
        ? formData.task_extend_plugin_keys
        : undefined,
    force_format: formData.force_format || false,
    thinking_to_content: formData.thinking_to_content || false,
    proxy: formData.proxy?.trim() || '',
    pass_through_body_enabled:
      formData.type !== CHANNEL_TYPE_ADVANCED_CUSTOM &&
      formData.pass_through_body_enabled === true,
    responses_websocket_enabled:
      (formData.type === 1 || formData.type === 57) &&
      formData.responses_websocket_enabled === true,
    system_prompt: formData.system_prompt || '',
    system_prompt_override: formData.system_prompt_override || false,
  }

  const protocol = normalizeHttpProtocol(formData.http_protocol)
  const shards =
    protocol === HTTP_PROTOCOL_HTTP1
      ? 1
      : normalizeHttp2ConnectionShards(formData.http2_connection_shards)

  // Omit defaults so unchanged channels keep equivalent JSON.
  if (protocol === HTTP_PROTOCOL_HTTP1) {
    settingObj.http_protocol = HTTP_PROTOCOL_HTTP1
  } else if (shards > 1) {
    settingObj.http2_connection_shards = shards
  }

  return JSON.stringify(settingObj)
}

/**
 * Build the settings JSON string (for type-specific config like vertex_key_type)
 */
function buildSettingsJSON(formData: ChannelFormValues): string {
  let settingsObj: Record<string, unknown> = {}

  // Try to parse existing settings first
  if (formData.settings && formData.settings !== '{}') {
    try {
      settingsObj = JSON.parse(formData.settings)
    } catch (error) {
      // eslint-disable-next-line no-console
      console.error('Failed to parse existing settings:', error)
    }
  }

  // Add vertex_key_type for Vertex AI channels (type 41)
  if (formData.type === 41) {
    settingsObj.vertex_key_type = formData.vertex_key_type || 'json'
  } else if ('vertex_key_type' in settingsObj) {
    delete settingsObj.vertex_key_type
  }

  if (formData.runtime_automatic_disable_override_enabled === true) {
    settingsObj.runtime_automatic_disable_override_enabled = true
    settingsObj.runtime_automatic_disable_status_codes =
      formData.runtime_automatic_disable_status_codes?.trim() || ''
    settingsObj.runtime_automatic_disable_keywords =
      formData.runtime_automatic_disable_keywords?.trim() || ''
  } else {
    delete settingsObj.runtime_automatic_disable_override_enabled
    delete settingsObj.runtime_automatic_disable_status_codes
    delete settingsObj.runtime_automatic_disable_keywords
  }
  delete settingsObj.automatic_disable_override_enabled
  delete settingsObj.automatic_disable_status_codes
  delete settingsObj.automatic_disable_keywords

  if (formData.empty_response_retry_override_enabled === true) {
    settingsObj.empty_response_retry_override_enabled = true
    settingsObj.empty_response_retry_enabled =
      formData.empty_response_retry_enabled === true
    settingsObj.empty_response_retry_in_place =
      formData.empty_response_retry_in_place === true
  } else {
    delete settingsObj.empty_response_retry_override_enabled
    delete settingsObj.empty_response_retry_enabled
    delete settingsObj.empty_response_retry_in_place
  }

  if (formData.automatic_retry_override_enabled === true) {
    settingsObj.automatic_retry_override_enabled = true
    settingsObj.automatic_retry_status_codes = formData.automatic_retry_status_codes?.trim() || ''
    settingsObj.automatic_retry_keywords = formData.automatic_retry_keywords?.trim() || ''
  } else {
    delete settingsObj.automatic_retry_override_enabled
    delete settingsObj.automatic_retry_status_codes
    delete settingsObj.automatic_retry_keywords
  }
  if (formData.type === 3 && formData.azure_responses_version) {
    settingsObj.azure_responses_version = formData.azure_responses_version
  } else if ('azure_responses_version' in settingsObj) {
    delete settingsObj.azure_responses_version
  }

  // Add enterprise account setting for OpenRouter (type 20)
  if (formData.type === 20) {
    settingsObj.openrouter_enterprise = formData.is_enterprise_account === true
  } else if ('openrouter_enterprise' in settingsObj) {
    delete settingsObj.openrouter_enterprise
  }

  // Add aws_key_type for AWS channels (type 33)
  if (formData.type === 33) {
    settingsObj.aws_key_type = formData.aws_key_type || 'ak_sk'
  } else if ('aws_key_type' in settingsObj) {
    delete settingsObj.aws_key_type
  }

  // Field passthrough controls:
  // - OpenAI, Anthropic, Codex, and New API: allow_service_tier
  // - OpenAI request fields: OpenAI, Codex, and New API
  // - Claude request fields: Anthropic and New API
  if (FIELD_PASSTHROUGH_TYPES.has(formData.type)) {
    settingsObj.allow_service_tier = formData.allow_service_tier === true
  } else if ('allow_service_tier' in settingsObj) {
    delete settingsObj.allow_service_tier
  }

  if (OPENAI_FIELD_PASSTHROUGH_TYPES.has(formData.type)) {
    settingsObj.disable_store = formData.disable_store === true
    settingsObj.allow_safety_identifier =
      formData.allow_safety_identifier === true
    settingsObj.allow_include_obfuscation =
      formData.allow_include_obfuscation === true
  } else {
    if ('disable_store' in settingsObj) {
      delete settingsObj.disable_store
    }
    if ('allow_safety_identifier' in settingsObj) {
      delete settingsObj.allow_safety_identifier
    }
    if ('allow_include_obfuscation' in settingsObj) {
      delete settingsObj.allow_include_obfuscation
    }
  }

  if (
    OPENAI_FIELD_PASSTHROUGH_TYPES.has(formData.type) ||
    CLAUDE_FIELD_PASSTHROUGH_TYPES.has(formData.type)
  ) {
    settingsObj.allow_inference_geo = formData.allow_inference_geo === true
  } else if ('allow_inference_geo' in settingsObj) {
    delete settingsObj.allow_inference_geo
  }

  if (CLAUDE_FIELD_PASSTHROUGH_TYPES.has(formData.type)) {
    settingsObj.allow_speed = formData.allow_speed === true
  } else if ('allow_speed' in settingsObj) {
    delete settingsObj.allow_speed
  }

  // Only the Anthropic adaptor supports forcing the Claude beta query.
  if (formData.type === 14) {
    settingsObj.claude_beta_query = formData.claude_beta_query === true
  } else if ('claude_beta_query' in settingsObj) {
    delete settingsObj.claude_beta_query
  }

  // Only the Ollama adaptor can switch chat completions to the OpenAI-compatible endpoint.
  if (formData.type === CHANNEL_TYPE_OLLAMA) {
    settingsObj.ollama_openai_chat = formData.ollama_openai_chat === true
  } else if ('ollama_openai_chat' in settingsObj) {
    delete settingsObj.ollama_openai_chat
  }

  settingsObj.disable_task_polling_sleep =
    formData.disable_task_polling_sleep === true

  // Upstream model update settings (for model-fetchable channel types)
  if (MODEL_FETCHABLE_TYPES.has(formData.type)) {
    settingsObj.upstream_model_update_check_enabled =
      formData.upstream_model_update_check_enabled === true
    settingsObj.upstream_model_update_auto_sync_enabled =
      settingsObj.upstream_model_update_check_enabled === true &&
      formData.upstream_model_update_auto_sync_enabled === true
    settingsObj.upstream_model_update_ignored_models = [
      ...new Set(
        String(formData.upstream_model_update_ignored_models || '')
          .split(',')
          .map((model) => model.trim())
          .filter(Boolean)
      ),
    ]
    if (
      !Array.isArray(settingsObj.upstream_model_update_last_detected_models) ||
      settingsObj.upstream_model_update_check_enabled !== true
    ) {
      settingsObj.upstream_model_update_last_detected_models = []
    }
    if (typeof settingsObj.upstream_model_update_last_check_time !== 'number') {
      settingsObj.upstream_model_update_last_check_time = 0
    }
  }

  if (formData.type === CHANNEL_TYPE_ADVANCED_CUSTOM) {
    const advancedCustomConfig = parseAdvancedCustomConfig(
      formData.advanced_custom
    )
    if (advancedCustomConfig) {
      settingsObj.advanced_custom = advancedCustomConfig
    }
  } else if ('advanced_custom' in settingsObj) {
    delete settingsObj.advanced_custom
  }

  if (formData.billing_query?.type === BILLING_QUERY_TYPE_NEW_API) {
    settingsObj.billing_query = {
      type: BILLING_QUERY_TYPE_NEW_API,
      base_url: normalizeBillingQueryBaseURL(formData.billing_query.base_url),
      bearer_token: formData.billing_query.bearer_token?.trim() || '',
      user_id: formData.billing_query.user_id?.trim() || '',
      use_api_key: formData.billing_query.use_api_key !== false,
    }
  } else if ('billing_query' in settingsObj) {
    delete settingsObj.billing_query
  }

  applyRatioProbeSettings(settingsObj, formData)

  const upscale = formData.image_upscale
  if (upscale?.enabled) {
    settingsObj.image_upscale = {
      enabled: true,
      target_channel_id:
        upscale.target_channel_id && upscale.target_channel_id > 0
          ? Math.trunc(upscale.target_channel_id)
          : 0,
      target_model: upscale.target_model?.trim() || '',
      models: [
        ...new Set(
          String(upscale.models || '')
            .split(',')
            .map((model) => model.trim())
            .filter(Boolean)
        ),
      ],
      on_error:
        upscale.on_error === IMAGE_UPSCALE_ON_ERROR_FAIL
          ? IMAGE_UPSCALE_ON_ERROR_FAIL
          : IMAGE_UPSCALE_ON_ERROR_FALLBACK,
      timeout_seconds:
        upscale.timeout_seconds && upscale.timeout_seconds > 0
          ? Math.trunc(upscale.timeout_seconds)
          : IMAGE_UPSCALE_DEFAULT_TIMEOUT_SECONDS,
    }
  } else if ('image_upscale' in settingsObj) {
    delete settingsObj.image_upscale
  }

  return JSON.stringify(settingsObj)
}

/**
 * Write the ratio probe config while preserving the probe state the backend
 * task owns (last probe time, last observed ratio, last message), so saving the
 * channel never erases the diagnostics shown in the editor.
 */
function applyRatioProbeSettings(
  settingsObj: Record<string, unknown>,
  formData: ChannelFormValues
): void {
  const probe = formData.ratio_probe
  if (!probe?.enabled) {
    if ('ratio_probe' in settingsObj) {
      delete settingsObj.ratio_probe
    }
    return
  }

  const previous = isJsonObjectValue(settingsObj.ratio_probe)
    ? settingsObj.ratio_probe
    : {}
  const nextProbe: Record<string, unknown> = {
    ...previous,
    enabled: true,
    source:
      probe.source === RATIO_PROBE_SOURCE_CUSTOM
        ? RATIO_PROBE_SOURCE_CUSTOM
        : RATIO_PROBE_SOURCE_FOLLOW_API,
    base_url:
      probe.source === RATIO_PROBE_SOURCE_CUSTOM
        ? normalizeBaseUrl(probe.base_url)
        : '',
    path: probe.path?.trim() || RATIO_PROBE_DEFAULT_PATH,
    group: probe.group?.trim() || '',
    use_api_key: probe.use_api_key !== false,
    authorization: probe.authorization?.trim() || '',
  }

  const maxRatio = parseRatioProbeBound(probe.max_group_ratio)
  const minRatio = parseRatioProbeBound(probe.min_group_ratio)
  if (typeof maxRatio === 'number') {
    nextProbe.max_group_ratio = maxRatio
  } else {
    delete nextProbe.max_group_ratio
  }
  if (typeof minRatio === 'number') {
    nextProbe.min_group_ratio = minRatio
  } else {
    delete nextProbe.min_group_ratio
  }

  settingsObj.ratio_probe = nextProbe
}

export function normalizeBillingQueryBaseURL(
  value: string | undefined
): string {
  return String(value || '')
    .trim()
    .replace(/\/+$/, '')
}

function normalizeBaseUrl(value: string | undefined): string {
  return String(value || '')
    .trim()
    .replace(/\/+$/, '')
}

/**
 * Transform form data to API payload for creating channel
 */
export function transformFormDataToCreatePayload(formData: ChannelFormValues): {
  mode: 'single' | 'batch' | 'multi_to_single'
  multi_key_mode?: 'random' | 'polling'
  batch_add_set_key_prefix_2_name?: boolean
  channel: Partial<Channel>
} {
  const mode = formData.multi_key_mode || 'single'

  const channel: Partial<Channel> = {
    name: formData.name,
    type: formData.type,
    base_url: normalizeBaseUrl(formData.base_url) || null,
    key: formData.key,
    openai_organization: formData.openai_organization || null,
    models: formData.models,
    group: formatGroups(formData.group),
    model_mapping: formData.model_mapping || null,
    priority: formData.priority || null,
    weight: formData.weight || null,
    test_model: formData.test_model || null,
    auto_ban: formData.auto_ban ?? 1,
    status: formData.status,
    status_code_mapping: formData.status_code_mapping || null,
    tag: formData.tag || null,
    remark: formData.remark || '',
    setting: buildSettingJSON(formData),
    param_override: formData.param_override || null,
    header_override: formData.header_override || null,
    settings: buildSettingsJSON(formData),
    other: formData.other || '',
  }

  // Clean up empty strings to null for optional fields
  Object.keys(channel).forEach((key) => {
    if (channel[key as keyof typeof channel] === '') {
      ;(channel as Record<string, unknown>)[key] = null
    }
  })

  return {
    mode,
    multi_key_mode:
      mode === 'multi_to_single' ? formData.multi_key_type : undefined,
    batch_add_set_key_prefix_2_name:
      mode === 'batch' ? formData.batch_add_set_key_prefix_2_name : undefined,
    channel,
  }
}

/**
 * Transform form data to API payload for updating channel
 */
export function transformFormDataToUpdatePayload(
  formData: ChannelFormValues,
  channelId: number
): Partial<Channel> {
  const payload: Partial<Channel> = {
    id: channelId,
    name: formData.name,
    type: formData.type,
    base_url: normalizeBaseUrl(formData.base_url) || null,
    openai_organization: formData.openai_organization || null,
    models: formData.models,
    group: formatGroups(formData.group),
    model_mapping: formData.model_mapping || null,
    priority: formData.priority ?? 0,
    weight: formData.weight ?? 0,
    test_model: formData.test_model || null,
    auto_ban: formData.auto_ban ?? 1,
    status_code_mapping: formData.status_code_mapping || null,
    tag: formData.tag || null,
    remark: formData.remark || '',
    setting: buildSettingJSON(formData),
    param_override: formData.param_override || null,
    header_override: formData.header_override || null,
    settings: buildSettingsJSON(formData),
    other: formData.other || '',
  }

  // Only include key if it was changed (not empty)
  if (formData.key && formData.key.trim()) {
    payload.key = formData.key
  }

  // Clean up empty strings to null for optional fields
  Object.keys(payload).forEach((key) => {
    if (payload[key as keyof typeof payload] === '') {
      ;(payload as Record<string, unknown>)[key] = null
    }
  })

  // Send explicit empty strings for nullable fields so GORM updates can clear them.
  payload.base_url = normalizeBaseUrl(formData.base_url) || ''
  payload.openai_organization = formData.openai_organization || ''
  payload.test_model = formData.test_model || ''
  payload.tag = formData.tag || ''
  payload.remark = formData.remark || ''
  payload.model_mapping = formData.model_mapping || ''
  payload.status_code_mapping = formData.status_code_mapping || ''
  payload.param_override = formData.param_override || ''
  payload.header_override = formData.header_override || ''

  return payload
}

// ============================================================================
// Validation Helpers
// ============================================================================

/**
 * Validate JSON string
 */
export function validateJSON(value: string): boolean {
  if (!value || value.trim() === '') return true
  try {
    JSON.parse(value)
    return true
  } catch {
    return false
  }
}

/**
 * Validate model mapping format
 */
export function validateModelMapping(value: string): boolean {
  if (!value || value.trim() === '') return true
  return validateJSON(value)
}

/**
 * Parse models string to array
 */
export function parseModels(models: string): string[] {
  if (!models) return []
  return models
    .split(',')
    .map((m) => m.trim())
    .filter((m) => m.length > 0)
}

/**
 * Parse groups string to array
 */
export function parseGroups(groups: string): string[] {
  if (!groups) return []
  return groups
    .split(',')
    .map((g) => g.trim())
    .filter((g) => g.length > 0)
}

/**
 * Format models array to string
 */
export function formatModels(models: string[]): string {
  return models.join(',')
}

/**
 * Format groups array to string
 */
export function formatGroups(groups: string[]): string {
  return groups.join(',')
}
