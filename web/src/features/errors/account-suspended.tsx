/*
Copyright (C) 2023-2026 xingguangcuican6666

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
*/

import { Link } from '@tanstack/react-router'
import { ShieldOff } from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { FadeIn } from '@/components/page-transition'
import { Button } from '@/components/ui/button'
import {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
} from '@/components/ui/empty'

/**
 * Full-page account state shown to a banned user.
 *
 * Serves both entry points with one composition: a user who is refused at
 * sign-in and a user whose account is disabled while they are using the
 * dashboard. The page is public — a banned user holds no usable session, so it
 * must not live behind the authenticated layout.
 *
 * The look is the project's own `anthropic` theme preset (warm cream canvas,
 * ink serif headline, a single clay accent) scoped to this page, so the
 * editorial styling holds even when the site runs a different preset. The
 * preset block only redefines CSS custom properties, and those inherit into
 * every element below this root, so `bg-background` / `text-primary` /
 * `font-serif` here resolve to the preset values without touching global
 * state or the visitor's theme choice.
 */
export function AccountSuspendedError() {
  const { t } = useTranslation()

  return (
    <main
      data-theme-preset='anthropic'
      className='bg-background text-foreground relative flex min-h-svh w-full flex-col items-center justify-center px-6 py-16'
    >
      <FadeIn className='w-full max-w-xl'>
        <Empty className='border-border/70 bg-card/60 rounded-2xl border p-8 shadow-xs sm:p-12'>
          <EmptyHeader className='max-w-md'>
            <EmptyMedia variant='default' className='mb-4'>
              <span
                aria-hidden='true'
                className='bg-primary/10 text-primary flex size-14 items-center justify-center rounded-full'
              >
                <ShieldOff className='size-6' />
              </span>
            </EmptyMedia>

            <p className='text-muted-foreground text-xs font-medium tracking-[0.2em] uppercase'>
              {t('Account status')}
            </p>

            {/* Same wording the backend returns in `auth.user_banned`, so a
                user who arrives here from a sign-in attempt reads one message
                rather than two different phrasings of the same verdict.
                A real heading rather than `EmptyTitle`: this is a full-page
                verdict, not a list's empty state, so the document outline
                needs it. */}
            <h1 className='text-foreground font-serif text-2xl leading-snug font-semibold text-pretty sm:text-3xl'>
              {t('Your account has been suspend!')}
            </h1>

            <span
              aria-hidden='true'
              className='bg-primary/70 mx-auto mt-1 h-px w-16'
            />

            <EmptyDescription className='text-base/relaxed'>
              {t(
                'Your existing sessions and API keys have been revoked, and this account can no longer place requests.'
              )}
            </EmptyDescription>
            <EmptyDescription className='text-sm'>
              {t(
                'If you believe this is a mistake, please the administrator of this site.'
              )}
            </EmptyDescription>
          </EmptyHeader>

          <EmptyContent className='mt-4 max-w-md'>
            <div className='flex w-full flex-col justify-center gap-2.5 sm:flex-row'>
              <Button
                size='lg'
                className='w-full sm:w-auto'
                render={<Link to='/sign-in' />}
              >
                {t('Sign in with another account')}
              </Button>
              <Button
                size='lg'
                variant='outline'
                className='w-full sm:w-auto'
                render={<Link to='/' />}
              >
                {t('Back to Home')}
              </Button>
            </div>
          </EmptyContent>
        </Empty>
      </FadeIn>
    </main>
  )
}
