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

Modified in this fork by xingguangcuican6666.
Modifications Copyright (C) 2023-2026 xingguangcuican6666
*/
import { zodResolver } from '@hookform/resolvers/zod'
import { useMemo, useRef } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import * as z from 'zod'

import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Separator } from '@/components/ui/separator'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { parseHttpStatusCodeRules } from '@/lib/http-status-code-rules'

import {
  SettingsForm,
  SettingsSwitchContent,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useResetForm } from '../hooks/use-reset-form'
import { useUpdateOption } from '../hooks/use-update-option'

const createRoutingReliabilitySchema = (
  t: (key: string, options?: Record<string, unknown>) => string
) =>
  z
    .object({
      RuntimeAutomaticDisableChannelEnabled: z.boolean(),
      RuntimeAutomaticDisableKeywords: z.string(),
      RuntimeAutomaticDisableStatusCodes: z.string(),
      EmptyResponseRetryEnabled: z.boolean(),
      EmptyResponseRetryInPlaceEnabled: z.boolean(),
    })
    .superRefine((values, ctx) => {
      const runtimeDisableParsed = parseHttpStatusCodeRules(
        values.RuntimeAutomaticDisableStatusCodes
      )
      if (!runtimeDisableParsed.ok) {
        ctx.addIssue({
          code: 'custom',
          path: ['RuntimeAutomaticDisableStatusCodes'],
          message: t('Invalid status code rules: {{tokens}}', {
            tokens: runtimeDisableParsed.invalidTokens.join(', '),
          }),
        })
      }
    })

type RoutingReliabilitySchema = ReturnType<
  typeof createRoutingReliabilitySchema
>
type RoutingReliabilityFormValues = z.output<RoutingReliabilitySchema>
type RoutingReliabilityFormInput = z.input<RoutingReliabilitySchema>

type RoutingReliabilitySectionProps = {
  defaultValues: {
    RuntimeAutomaticDisableChannelEnabled: boolean
    RuntimeAutomaticDisableKeywords: string
    RuntimeAutomaticDisableStatusCodes: string
    EmptyResponseRetryEnabled: boolean
    EmptyResponseRetryInPlaceEnabled: boolean
  }
}

function normalizeLineEndings(value: string) {
  return value.replaceAll('\r\n', '\n')
}

type NormalizedRoutingReliabilityValues = {
  RuntimeAutomaticDisableChannelEnabled: boolean
  RuntimeAutomaticDisableKeywords: string
  RuntimeAutomaticDisableStatusCodes: string
  EmptyResponseRetryEnabled: boolean
  EmptyResponseRetryInPlaceEnabled: boolean
}

const buildFormDefaults = (
  defaults: RoutingReliabilitySectionProps['defaultValues']
): RoutingReliabilityFormInput => ({
  RuntimeAutomaticDisableChannelEnabled:
    defaults.RuntimeAutomaticDisableChannelEnabled,
  RuntimeAutomaticDisableKeywords: normalizeLineEndings(
    defaults.RuntimeAutomaticDisableKeywords ?? ''
  ),
  RuntimeAutomaticDisableStatusCodes:
    defaults.RuntimeAutomaticDisableStatusCodes ?? '',
  EmptyResponseRetryEnabled: defaults.EmptyResponseRetryEnabled,
  EmptyResponseRetryInPlaceEnabled: defaults.EmptyResponseRetryInPlaceEnabled,
})

const normalizeDefaults = (
  defaults: RoutingReliabilitySectionProps['defaultValues']
): NormalizedRoutingReliabilityValues => ({
  RuntimeAutomaticDisableChannelEnabled:
    defaults.RuntimeAutomaticDisableChannelEnabled,
  RuntimeAutomaticDisableKeywords: normalizeLineEndings(
    defaults.RuntimeAutomaticDisableKeywords ?? ''
  ),
  RuntimeAutomaticDisableStatusCodes: parseHttpStatusCodeRules(
    defaults.RuntimeAutomaticDisableStatusCodes ?? ''
  ).normalized,
  EmptyResponseRetryEnabled: defaults.EmptyResponseRetryEnabled,
  EmptyResponseRetryInPlaceEnabled: defaults.EmptyResponseRetryInPlaceEnabled,
})

const normalizeFormValues = (
  values: RoutingReliabilityFormValues
): NormalizedRoutingReliabilityValues => ({
  RuntimeAutomaticDisableChannelEnabled:
    values.RuntimeAutomaticDisableChannelEnabled,
  RuntimeAutomaticDisableKeywords: normalizeLineEndings(
    values.RuntimeAutomaticDisableKeywords
  ),
  RuntimeAutomaticDisableStatusCodes: parseHttpStatusCodeRules(
    values.RuntimeAutomaticDisableStatusCodes
  ).normalized,
  EmptyResponseRetryEnabled: values.EmptyResponseRetryEnabled,
  EmptyResponseRetryInPlaceEnabled: values.EmptyResponseRetryInPlaceEnabled,
})

export function RoutingReliabilitySection({
  defaultValues,
}: RoutingReliabilitySectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const routingReliabilitySchema = createRoutingReliabilitySchema(t)
  const baselineRef = useRef<NormalizedRoutingReliabilityValues>(
    normalizeDefaults(defaultValues)
  )

  const formDefaults = useMemo(
    () => buildFormDefaults(defaultValues),
    [defaultValues]
  )

  const form = useForm<
    RoutingReliabilityFormInput,
    unknown,
    RoutingReliabilityFormValues
  >({
    resolver: zodResolver(routingReliabilitySchema),
    defaultValues: formDefaults,
  })

  useResetForm(form, formDefaults)

  const runtimeAutoDisableStatusCodes = form.watch(
    'RuntimeAutomaticDisableStatusCodes'
  )
  const runtimeAutoDisableParsed = useMemo(
    () => parseHttpStatusCodeRules(runtimeAutoDisableStatusCodes),
    [runtimeAutoDisableStatusCodes]
  )

  const onSubmit = async (values: RoutingReliabilityFormValues) => {
    const normalized = normalizeFormValues(values)
    const updates = (
      Object.keys(normalized) as Array<keyof NormalizedRoutingReliabilityValues>
    ).filter((key) => normalized[key] !== baselineRef.current[key])

    if (updates.length === 0) {
      toast.info(t('No changes to save'))
      return
    }

    for (const key of updates) {
      const value = normalized[key]
      await updateOption.mutateAsync({
        key,
        value,
      })
    }

    baselineRef.current = normalized
  }

  return (
    <SettingsSection title={t('Routing Reliability')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            isSaving={updateOption.isPending}
          />

          <div className='flex min-w-0 flex-col gap-4'>
            <div className='flex flex-col gap-1'>
              <h4 className='text-sm font-medium'>{t('Request retry')}</h4>
            </div>
            <div className='grid min-w-0 gap-6 xl:grid-cols-[minmax(12rem,24rem)_minmax(0,1fr)]'>
              <FormField
                control={form.control}
                name='EmptyResponseRetryEnabled'
                render={({ field }) => (
                  <SettingsSwitchItem className='xl:col-span-2'>
                    <SettingsSwitchContent>
                      <FormLabel>{t('Retry empty responses')}</FormLabel>
                      <FormDescription>
                        {t(
                          'Treat an upstream 200 that carries no content and no output tokens as a failure. The reply is withheld from the client and the request is retried.'
                        )}
                      </FormDescription>
                    </SettingsSwitchContent>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                  </SettingsSwitchItem>
                )}
              />
              <FormField
                control={form.control}
                name='EmptyResponseRetryInPlaceEnabled'
                render={({ field }) => (
                  <SettingsSwitchItem className='xl:col-span-2'>
                    <SettingsSwitchContent>
                      <FormLabel>{t('Retry on the same channel')}</FormLabel>
                      <FormDescription>
                        {t(
                          'Retry an empty response on the same channel and key until the retry limit is reached, instead of switching to another channel.'
                        )}
                      </FormDescription>
                    </SettingsSwitchContent>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                  </SettingsSwitchItem>
                )}
              />
            </div>
          </div>

          <Separator />

          <div className='flex min-w-0 flex-col gap-4'>
            <div className='flex flex-col gap-1'>
              <h4 className='text-sm font-medium'>
                {t('Runtime auto-disable')}
              </h4>
              <p className='text-sm text-muted-foreground'>
                {t(
                  'Applies only to errors returned during real user relay requests.'
                )}
              </p>
            </div>
            <div className='grid min-w-0 gap-6 lg:grid-cols-2'>
              <FormField
                control={form.control}
                name='RuntimeAutomaticDisableChannelEnabled'
                render={({ field }) => (
                  <SettingsSwitchItem>
                    <SettingsSwitchContent>
                      <FormLabel>{t('Enable runtime auto-disable')}</FormLabel>
                      <FormDescription>
                        {t(
                          'Disable the current key in a multi-key channel, or the whole channel otherwise, when a runtime rule matches.'
                        )}
                      </FormDescription>
                    </SettingsSwitchContent>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                  </SettingsSwitchItem>
                )}
              />
              <FormField
                control={form.control}
                name='RuntimeAutomaticDisableStatusCodes'
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>{t('Runtime error status codes')}</FormLabel>
                    <FormControl>
                      <Input
                        placeholder={t('e.g. 401, 403, 429, 500-599')}
                        value={field.value}
                        onChange={field.onChange}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'Accepts comma-separated status codes and inclusive ranges.'
                      )}
                      {runtimeAutoDisableParsed.ok &&
                        runtimeAutoDisableParsed.normalized &&
                        runtimeAutoDisableParsed.normalized !==
                          field.value.trim() && (
                          <span className='ml-1'>
                            {t('Normalized:')}{' '}
                            {runtimeAutoDisableParsed.normalized}
                          </span>
                        )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
              <FormField
                control={form.control}
                name='RuntimeAutomaticDisableKeywords'
                render={({ field }) => (
                  <FormItem className='lg:col-span-2'>
                    <FormLabel>{t('Runtime error keywords')}</FormLabel>
                    <FormControl>
                      <Textarea
                        rows={6}
                        placeholder={t('one keyword per line')}
                        {...field}
                      />
                    </FormControl>
                    <FormDescription>
                      {t(
                        'A case-insensitive match in an upstream error returned to a real user request triggers runtime auto-disable.'
                      )}
                    </FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            </div>
          </div>
        </SettingsForm>
      </Form>
    </SettingsSection>
  )
}