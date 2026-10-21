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
import { Link } from '@tanstack/react-router'
import { ArrowRight, BookOpen, CreditCard, KeyRound, LayoutGrid } from 'lucide-react'
import { useTranslation } from 'react-i18next'

interface QuickAction {
  title: string
  description: string
  to: string
  icon: typeof KeyRound
}

export function QuickActionsPanel() {
  const { t } = useTranslation()

  const actions: QuickAction[] = [
    {
      title: t('Create API Key'),
      description: t('Create a key for your app or service'),
      to: '/keys',
      icon: KeyRound,
    },
    {
      title: t('API Documentation'),
      description: t('Read the full API integration guide'),
      to: '/docs',
      icon: BookOpen,
    },
    {
      title: t('Model Square'),
      description: t('Browse available models and prices'),
      to: '/pricing',
      icon: LayoutGrid,
    },
    {
      title: t('Top up'),
      description: t('Add balance to keep services running'),
      to: '/wallet',
      icon: CreditCard,
    },
  ]

  return (
    <div className='bg-card border-border/70 flex h-full flex-col rounded-2xl border p-5 shadow-xs'>
      <h3 className='mb-3 text-base font-semibold'>{t('Quick actions')}</h3>
      <div className='flex flex-col gap-2'>
        {actions.map((action) => {
          const Icon = action.icon
          return (
            <Link
              key={action.to}
              to={action.to}
              className='border-border/50 hover:border-primary/30 hover:bg-muted/40 group flex items-center gap-3 rounded-xl border p-3 transition-colors'
            >
              <span className='bg-primary/10 text-primary flex size-9 shrink-0 items-center justify-center rounded-lg'>
                <Icon className='size-4' strokeWidth={1.7} />
              </span>
              <div className='min-w-0 flex-1'>
                <p className='text-sm font-medium'>{action.title}</p>
                <p className='text-muted-foreground truncate text-[11px]'>
                  {action.description}
                </p>
              </div>
              <ArrowRight className='text-muted-foreground size-4 shrink-0 transition-transform group-hover:translate-x-0.5' />
            </Link>
          )
        })}
      </div>
    </div>
  )
}
