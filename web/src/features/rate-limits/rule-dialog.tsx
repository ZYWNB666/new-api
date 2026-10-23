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
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'

import { Dialog } from '@/components/dialog'
import { Button } from '@/components/ui/button'
import { Combobox } from '@/components/ui/combobox'
import {
  Form,
  FormControl,
  FormField,
  FormItem,
  FormLabel,
  FormMessage,
} from '@/components/ui/form'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'

import { ruleSchema, type Rule } from './api'
import { TargetPicker } from './target-picker'

export function RuleDialog(props: {
  rule: Rule | null
  pending: boolean
  onClose: () => void
  onSave: (rule: Rule) => Promise<void>
}) {
  const { t } = useTranslation()
  const form = useForm<Rule>({
    resolver: zodResolver(ruleSchema),
    defaultValues: props.rule ?? {
      id: crypto.randomUUID(),
      name: '',
      scope: 'user',
      subject_id: 0,
      model: '',
      channel_ids: [],
      rpm: 0,
      tpm: 0,
      concurrent: 0,
      enabled: true,
    },
  })
  const scope = form.watch('scope')
  const target = form.watch(scope === 'provider' ? 'channel_ids' : 'subject_id')
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !props.pending) props.onClose()
      }}
      title={props.rule ? t('Edit rate limit') : t('Add rate limit')}
      description={t('Zero means unlimited. Set at least one positive limit.')}
      footer={
        <>
          <Button
            variant='outline'
            disabled={props.pending}
            onClick={props.onClose}
          >
            {t('Cancel')}
          </Button>
          <Button type='submit' form='gateway-rule' disabled={props.pending}>
            {t('Save')}
          </Button>
        </>
      }
    >
      <Form {...form}>
        <form
          id='gateway-rule'
          onSubmit={form.handleSubmit(async (rule) => {
            try {
              await props.onSave(rule)
            } catch {
              /* Mutation reports the failure and keeps the editor open. */
            }
          })}
          className='space-y-4'
        >
          <FormField
            control={form.control}
            name='name'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Name')}</FormLabel>
                <FormControl>
                  <Input {...field} maxLength={128} />
                </FormControl>
                <FormMessage />
              </FormItem>
            )}
          />
          <FormField
            control={form.control}
            name='scope'
            render={({ field }) => (
              <FormItem>
                <FormLabel>{t('Scope')}</FormLabel>
                <Combobox
                  aria-label={t('Scope')}
                  options={[
                    { value: 'user', label: t('Users') },
                    { value: 'api_key', label: t('API Keys') },
                    { value: 'user_model', label: t('User + model') },
                    { value: 'provider', label: t('Supplier pool') },
                  ]}
                  value={field.value}
                  disabled={!!props.rule}
                  onValueChange={(value) => {
                    field.onChange(value ?? 'user')
                    form.setValue('subject_id', 0)
                    form.setValue('channel_ids', [])
                    form.setValue('model', '')
                  }}
                />
                <FormMessage />
              </FormItem>
            )}
          />
          <div className='space-y-2'>
            <Label htmlFor='limit-target'>{t('Target')}</Label>
            <TargetPicker
              key={scope}
              scope={scope}
              value={Array.isArray(target) ? target : [target].filter(Boolean)}
              disabled={!!props.rule}
              onChange={(ids) => {
                if (scope === 'provider') form.setValue('channel_ids', ids)
                else form.setValue('subject_id', ids[0] ?? 0)
              }}
            />
            {form.formState.errors.subject_id && (
              <p role='alert' className='text-destructive text-sm'>
                {t('Select a target')}
              </p>
            )}
          </div>
          {scope === 'user_model' && (
            <FormField
              control={form.control}
              name='model'
              render={({ field }) => (
                <FormItem>
                  <FormLabel>{t('Model')}</FormLabel>
                  <FormControl>
                    <Input {...field} disabled={!!props.rule} maxLength={256} />
                  </FormControl>
                  <p className='text-muted-foreground text-xs'>
                    {t('Match the exact model name requested by the client.')}
                  </p>
                  <FormMessage />
                </FormItem>
              )}
            />
          )}
          <div className='grid gap-4 sm:grid-cols-3'>
            {(['rpm', 'tpm', 'concurrent'] as const).map((dimension) => (
              <FormField
                key={dimension}
                control={form.control}
                name={dimension}
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>
                      {dimension === 'concurrent'
                        ? t('Concurrency')
                        : dimension.toUpperCase()}
                    </FormLabel>
                    <FormControl>
                      <Input
                        {...field}
                        type='number'
                        min={0}
                        max={1_000_000_000}
                        step={1}
                        onChange={(event) =>
                          field.onChange(
                            event.target.value === ''
                              ? 0
                              : Number(event.target.value)
                          )
                        }
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            ))}
          </div>
          <FormField
            control={form.control}
            name='enabled'
            render={({ field }) => (
              <FormItem className='flex items-center justify-between'>
                <FormLabel>{t('Enabled')}</FormLabel>
                <FormControl>
                  <Switch
                    checked={field.value}
                    onCheckedChange={field.onChange}
                  />
                </FormControl>
              </FormItem>
            )}
          />
        </form>
      </Form>
    </Dialog>
  )
}
