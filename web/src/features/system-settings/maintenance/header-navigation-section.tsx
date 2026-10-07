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
import { zodResolver } from '@hookform/resolvers/zod'
import type { TFunction } from 'i18next'
import { useEffect, useMemo } from 'react'
import { useForm, useWatch } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
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
import { Switch } from '@/components/ui/switch'
import { isValidCustomNavHref } from '@/lib/nav-modules'

import {
  SettingsControlChildren,
  SettingsForm,
  SettingsSwitchContent,
  SettingsControlGroup,
  SettingsSwitchItem,
} from '../components/settings-form-layout'
import { SettingsPageFormActions } from '../components/settings-page-context'
import { SettingsSection } from '../components/settings-section'
import { useUpdateOption } from '../hooks/use-update-option'
import {
  HEADER_NAV_DEFAULT,
  type HeaderNavModulesConfig,
  serializeHeaderNavModules,
} from './config'
import { customNavItemSchema } from './custom-nav-item-schema'
import { CustomNavItemsEditor } from './custom-nav-items-editor'

const getHeaderNavSchema = (t: TFunction) =>
  z.object({
    home: z.boolean(),
    console: z.boolean(),
    pricingEnabled: z.boolean(),
    pricingRequireAuth: z.boolean(),
    rankingsEnabled: z.boolean(),
    rankingsRequireAuth: z.boolean(),
    docs: z.boolean(),
    // Shared with the System Information section: the same option, empty
    // meaning "use the default documentation".
    docsLink: z
      .string()
      .trim()
      .refine(
        (value) => value === '' || isValidCustomNavHref(value, 'external'),
        t('Enter a valid http(s) URL.')
      ),
    about: z.boolean(),
    customItems: z.array(customNavItemSchema),
  })

type HeaderNavFormValues = z.infer<ReturnType<typeof getHeaderNavSchema>>

type HeaderNavigationSectionProps = {
  config: HeaderNavModulesConfig
  initialSerialized: string
  /** Current `general_setting.docs_link` option value. */
  docsLink: string
}

const toFormValues = (
  config: HeaderNavModulesConfig,
  docsLink: string
): HeaderNavFormValues => ({
  home:
    config.home === undefined ? HEADER_NAV_DEFAULT.home : Boolean(config.home),
  console:
    config.console === undefined
      ? HEADER_NAV_DEFAULT.console
      : Boolean(config.console),
  pricingEnabled:
    config.pricing?.enabled === undefined
      ? HEADER_NAV_DEFAULT.pricing.enabled
      : Boolean(config.pricing.enabled),
  pricingRequireAuth:
    config.pricing?.requireAuth === undefined
      ? HEADER_NAV_DEFAULT.pricing.requireAuth
      : Boolean(config.pricing.requireAuth),
  rankingsEnabled:
    config.rankings?.enabled === undefined
      ? HEADER_NAV_DEFAULT.rankings.enabled
      : Boolean(config.rankings.enabled),
  rankingsRequireAuth:
    config.rankings?.requireAuth === undefined
      ? HEADER_NAV_DEFAULT.rankings.requireAuth
      : Boolean(config.rankings.requireAuth),
  docs:
    config.docs === undefined ? HEADER_NAV_DEFAULT.docs : Boolean(config.docs),
  docsLink,
  about:
    config.about === undefined
      ? HEADER_NAV_DEFAULT.about
      : Boolean(config.about),
  customItems: config.customItems,
})

export function HeaderNavigationSection(props: HeaderNavigationSectionProps) {
  const { t } = useTranslation()
  const updateOption = useUpdateOption()
  const schema = useMemo(() => getHeaderNavSchema(t), [t])
  const formDefaults = useMemo(
    () => toFormValues(props.config, props.docsLink),
    [props.config, props.docsLink]
  )

  const form = useForm<HeaderNavFormValues>({
    resolver: zodResolver(schema),
    defaultValues: formDefaults,
  })
  const customItems = useWatch({ control: form.control, name: 'customItems' })

  useEffect(() => {
    form.reset(formDefaults)
  }, [formDefaults, form])

  const onSubmit = async (values: HeaderNavFormValues) => {
    const payload: HeaderNavModulesConfig = {
      ...props.config,
      home: values.home,
      console: values.console,
      docs: values.docs,
      about: values.about,
      pricing: {
        ...(props.config.pricing ?? HEADER_NAV_DEFAULT.pricing),
        enabled: values.pricingEnabled,
        requireAuth: values.pricingRequireAuth,
      },
      rankings: {
        ...(props.config.rankings ?? HEADER_NAV_DEFAULT.rankings),
        enabled: values.rankingsEnabled,
        requireAuth: values.rankingsRequireAuth,
      },
      customItems: values.customItems,
    }

    const serialized = serializeHeaderNavModules(payload)
    if (serialized !== props.initialSerialized) {
      await updateOption.mutateAsync({
        key: 'HeaderNavModules',
        value: serialized,
      })
    }

    if (values.docsLink !== props.docsLink.trim()) {
      await updateOption.mutateAsync({
        key: 'general_setting.docs_link',
        value: values.docsLink,
      })
    }
  }

  // The documentation link is a site-wide option shared with System
  // Information, so resetting the navigation defaults leaves it as typed.
  const resetToDefault = () => {
    form.reset(toFormValues(HEADER_NAV_DEFAULT, form.getValues('docsLink')))
  }

  const simpleModules: Array<{
    key: 'home' | 'console' | 'about'
    title: string
    description: string
  }> = [
    {
      key: 'home',
      title: t('Home'),
      description: t('Landing page with system overview.'),
    },
    {
      key: 'console',
      title: t('Console'),
      description: t('User dashboard and quota controls.'),
    },
    {
      key: 'about',
      title: t('About'),
      description: t('Static page describing the platform.'),
    },
  ]

  const accessModules: Array<{
    enabledKey: 'pricingEnabled' | 'rankingsEnabled'
    requireAuthKey: 'pricingRequireAuth' | 'rankingsRequireAuth'
    requireAuthDependsOn: 'pricingEnabled' | 'rankingsEnabled'
    title: string
    description: string
    requireAuthTitle: string
    requireAuthDescription: string
  }> = [
    {
      enabledKey: 'pricingEnabled',
      requireAuthKey: 'pricingRequireAuth',
      requireAuthDependsOn: 'pricingEnabled',
      title: t('Model Square'),
      description: t('Public model catalog and pricing page.'),
      requireAuthTitle: t('Require login to view models'),
      requireAuthDescription: t(
        'Visitors must authenticate before accessing the pricing directory.'
      ),
    },
    {
      enabledKey: 'rankingsEnabled',
      requireAuthKey: 'rankingsRequireAuth',
      requireAuthDependsOn: 'rankingsEnabled',
      title: t('Rankings'),
      description: t('Public rankings page based on live usage data.'),
      requireAuthTitle: t('Require login to view rankings'),
      requireAuthDescription: t(
        'Visitors must authenticate before accessing the rankings page.'
      ),
    },
  ]

  return (
    <SettingsSection title={t('Header navigation')}>
      <Form {...form}>
        <SettingsForm onSubmit={form.handleSubmit(onSubmit)}>
          <SettingsPageFormActions
            onSave={form.handleSubmit(onSubmit)}
            onReset={resetToDefault}
            isSaving={updateOption.isPending}
            resetLabel='Reset to default'
            saveLabel='Save navigation'
          />
          <div className='grid gap-4 md:grid-cols-2'>
            {simpleModules.map((module) => (
              <FormField
                key={module.key}
                control={form.control}
                name={module.key}
                render={({ field }) => (
                  <SettingsSwitchItem>
                    <SettingsSwitchContent>
                      <FormLabel>{module.title}</FormLabel>
                      <FormDescription>{module.description}</FormDescription>
                    </SettingsSwitchContent>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                    <FormMessage />
                  </SettingsSwitchItem>
                )}
              />
            ))}
          </div>

          <div className='grid gap-4 lg:grid-cols-2'>
            {accessModules.map((module) => (
              <SettingsControlGroup key={module.enabledKey}>
                <FormField
                  control={form.control}
                  name={module.enabledKey}
                  render={({ field }) => (
                    <SettingsSwitchItem>
                      <SettingsSwitchContent>
                        <FormLabel>{module.title}</FormLabel>
                        <FormDescription>{module.description}</FormDescription>
                      </SettingsSwitchContent>
                      <FormControl>
                        <Switch
                          checked={field.value}
                          onCheckedChange={field.onChange}
                        />
                      </FormControl>
                      <FormMessage />
                    </SettingsSwitchItem>
                  )}
                />

                <FormField
                  control={form.control}
                  name={module.requireAuthKey}
                  render={({ field }) => (
                    <SettingsControlChildren>
                      <SettingsSwitchItem className='py-2'>
                        <SettingsSwitchContent>
                          <FormLabel>{module.requireAuthTitle}</FormLabel>
                          <FormDescription>
                            {module.requireAuthDescription}
                          </FormDescription>
                        </SettingsSwitchContent>
                        <FormControl>
                          <Switch
                            checked={field.value}
                            onCheckedChange={field.onChange}
                            disabled={!form.watch(module.requireAuthDependsOn)}
                          />
                        </FormControl>
                        <FormMessage />
                      </SettingsSwitchItem>
                    </SettingsControlChildren>
                  )}
                />
              </SettingsControlGroup>
            ))}

            <SettingsControlGroup>
              <FormField
                control={form.control}
                name='docs'
                render={({ field }) => (
                  <SettingsSwitchItem>
                    <SettingsSwitchContent>
                      <FormLabel>{t('Docs')}</FormLabel>
                      <FormDescription>
                        {t('Documentation or external knowledge base.')}
                      </FormDescription>
                    </SettingsSwitchContent>
                    <FormControl>
                      <Switch
                        checked={field.value}
                        onCheckedChange={field.onChange}
                      />
                    </FormControl>
                    <FormMessage />
                  </SettingsSwitchItem>
                )}
              />

              <FormField
                control={form.control}
                name='docsLink'
                render={({ field }) => (
                  <SettingsControlChildren>
                    <FormItem>
                      <FormLabel>{t('Documentation Link')}</FormLabel>
                      <FormControl>
                        <Input
                          placeholder={t('https://docs.example.com')}
                          {...field}
                        />
                      </FormControl>
                      <FormDescription>
                        {t('Link to your documentation site')}
                      </FormDescription>
                      <FormMessage />
                    </FormItem>
                  </SettingsControlChildren>
                )}
              />
            </SettingsControlGroup>
          </div>
        </SettingsForm>
      </Form>

      {/* Outside the form above: the dialog has its own <form>, and submit events bubble through React portals. */}
      <CustomNavItemsEditor
        items={customItems}
        onChange={(items) =>
          form.setValue('customItems', items, { shouldDirty: true })
        }
      />
    </SettingsSection>
  )
}
