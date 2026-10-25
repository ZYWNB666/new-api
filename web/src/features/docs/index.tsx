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
import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import {
  ArrowUpRight,
  BookOpen,
  KeyRound,
  Search,
  ShieldAlert,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'

import { CopyButton } from '@/components/copy-button'
import { StaticDataTable } from '@/components/data-table/static/static-data-table'
import { Footer } from '@/components/layout/components/footer'
import { PublicLayout } from '@/components/layout/components/public-layout'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'

const CURL_EXAMPLE = `curl https://YOUR_GATEWAY_HOST/v1/chat/completions \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "YOUR_MODEL_NAME",
    "messages": [{"role": "user", "content": "Hello"}]
  }'`

const SDK_EXAMPLE = `from openai import OpenAI
import os

client = OpenAI(
    api_key=os.environ["NEW_API_KEY"],
    base_url="https://YOUR_GATEWAY_HOST/v1",
)
response = client.chat.completions.create(
    model="YOUR_MODEL_NAME",
    messages=[{"role": "user", "content": "Hello"}],
)
print(response.choices[0].message.content)`

const STREAM_EXAMPLE = `stream = client.chat.completions.create(
    model="YOUR_MODEL_NAME",
    messages=[{"role": "user", "content": "Hello"}],
    stream=True,
)
for chunk in stream:
    if chunk.choices and chunk.choices[0].delta.content:
        print(chunk.choices[0].delta.content, end="")`

function DocsExample(props: { code: string; label: string }) {
  const { t } = useTranslation()
  return (
    <div className='border-border bg-muted/40 min-w-0 overflow-hidden rounded-xl border'>
      <div className='flex items-center justify-between border-b px-4 py-2'>
        <span className='text-muted-foreground text-xs font-medium'>
          {props.label}
        </span>
        <CopyButton value={props.code} aria-label={t('Copy example')} size='sm'>
          {t('Copy')}
        </CopyButton>
      </div>
      <pre
        tabIndex={0}
        aria-label={props.label}
        className='max-w-full overflow-x-auto p-4 text-xs leading-6 sm:text-sm'
      >
        <code>{props.code}</code>
      </pre>
    </div>
  )
}

interface DocSection {
  id: string
  title: string
}

interface DocGroup {
  title: string
  sections: DocSection[]
}

export function Docs() {
  const { t } = useTranslation()
  const [query, setQuery] = useState('')

  const sections: DocSection[] = [
    { id: 'credentials', title: t('API credentials') },
    { id: 'base-url', title: 'Base URL' },
    { id: 'quickstart', title: t('Quick start') },
    { id: 'streaming', title: t('Streaming') },
    { id: 'models', title: t('Model selection') },
    { id: 'errors', title: t('Common errors') },
    { id: 'request-id', title: 'Request ID' },
  ]

  const groups: DocGroup[] = [
    {
      title: t('Getting started'),
      sections: sections.slice(0, 3),
    },
    {
      title: t('API reference'),
      sections: sections.slice(3, 5),
    },
    {
      title: t('Help'),
      sections: sections.slice(5),
    },
  ]

  const normalizedQuery = query.trim().toLowerCase()
  const filteredGroups = groups
    .map((group) => ({
      ...group,
      sections: group.sections.filter((section) =>
        section.title.toLowerCase().includes(normalizedQuery)
      ),
    }))
    .filter((group) => group.sections.length > 0)
  const errors = [
    {
      code: '401',
      text: t(
        'Check your API key and its expiration. Account passwords are not API keys.'
      ),
    },
    {
      code: '403',
      text: t(
        'Check token permissions, model restrictions, and your group access.'
      ),
    },
    {
      code: '429',
      text: t(
        'Check rate limits and quota. Retry with backoff when appropriate.'
      ),
    },
    {
      code: '5xx',
      text: t(
        'The gateway or upstream service failed. Save the Request ID before retrying.'
      ),
    },
  ]

  return (
    <PublicLayout showMainContainer={false}>
      <main className='public-surface px-4 pt-24 pb-16 sm:px-6'>
        <div className='mx-auto max-w-7xl'>
          <div className='grid items-start gap-8 lg:grid-cols-[15rem_minmax(0,1fr)] xl:grid-cols-[15rem_minmax(0,1fr)_13rem]'>
            {/* Left: grouped documentation navigation with search */}
            <aside className='lg:sticky lg:top-24 lg:max-h-[calc(100vh-8rem)] lg:overflow-y-auto'>
              <div className='relative mb-4'>
                <Search
                  aria-hidden='true'
                  className='text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2'
                />
                <Input
                  type='search'
                  value={query}
                  onChange={(e) => setQuery(e.target.value)}
                  placeholder={t('Search documentation')}
                  aria-label={t('Search documentation')}
                  className='pl-9'
                />
              </div>
              <nav
                aria-label={t('Documentation navigation')}
                className='flex gap-2 overflow-x-auto lg:flex-col lg:gap-1'
              >
                {filteredGroups.map((group) => (
                  <div key={group.title} className='shrink-0 lg:mb-4'>
                    <p className='text-muted-foreground/70 mb-1.5 px-3 text-[11px] font-semibold tracking-wider uppercase'>
                      {group.title}
                    </p>
                    <ul className='flex gap-1 lg:flex-col'>
                      {group.sections.map((section) => (
                        <li key={section.id}>
                          <a
                            href={`#${section.id}`}
                            className='text-muted-foreground hover:bg-primary/10 hover:text-primary focus-visible:outline-ring block shrink-0 rounded-lg px-3 py-2 text-sm transition-colors'
                          >
                            {section.title}
                          </a>
                        </li>
                      ))}
                    </ul>
                  </div>
                ))}
                {filteredGroups.length === 0 && (
                  <p className='text-muted-foreground px-3 py-2 text-sm'>
                    {t('No results')}
                  </p>
                )}
              </nav>
            </aside>

            {/* Center: article */}
            <article className='bg-card min-w-0 rounded-2xl border p-5 shadow-xs sm:p-8'>
              <div className='text-muted-foreground mb-4 flex items-center gap-2 text-xs'>
                <BookOpen aria-hidden='true' className='size-3.5' />
                <span>{t('Documentation')}</span>
                <span aria-hidden='true'>/</span>
                <span className='text-foreground'>{t('Quick start')}</span>
              </div>
              <h1 className='text-3xl font-bold tracking-tight sm:text-4xl'>
                {t('Build with your AI gateway')}
              </h1>
              <p className='text-muted-foreground mt-4 leading-7'>
                {t(
                  'A practical guide to API credentials, compatible requests, and troubleshooting.'
                )}
              </p>
              <div className='mt-6 flex flex-wrap gap-3'>
                <Button render={<Link to='/keys' />}>
                  <KeyRound className='size-4' />
                  {t('Get API Key')}
                </Button>
                <Button variant='outline' render={<Link to='/pricing' />}>
                  {t('View Pricing')}
                </Button>
              </div>

              <div className='mt-10 space-y-10'>
              <section id='credentials' className='docs-section space-y-4'>
                <h2 className='text-xl font-semibold'>
                  {t('API credentials')}
                </h2>
                <p className='text-muted-foreground leading-7'>
                  {t(
                    'Your account signs you into the console. An API key authorizes model requests; never use your account password as a Bearer token.'
                  )}
                </p>
                <p className='text-muted-foreground leading-7'>
                  {t(
                    'Create a key in the console, restrict its models and quota as needed, and keep it on your server. Never publish keys in browser code or logs.'
                  )}
                </p>
                <Button variant='outline' render={<Link to='/keys' />}>
                  {t('API Keys')}
                </Button>
                <div className='border-amber-500/30 bg-amber-500/5 flex items-start gap-3 rounded-xl border p-4'>
                  <ShieldAlert
                    aria-hidden='true'
                    className='mt-0.5 size-5 shrink-0 text-amber-600 dark:text-amber-400'
                  />
                  <div>
                    <p className='text-sm font-semibold'>
                      {t('Keep your API key safe')}
                    </p>
                    <p className='text-muted-foreground mt-1 text-sm leading-relaxed'>
                      {t(
                        'Never expose your API key in browser-side code or public repositories.'
                      )}
                    </p>
                  </div>
                </div>
              </section>
              <section id='base-url' className='docs-section space-y-4'>
                <h2 className='text-xl font-semibold'>Base URL</h2>
                <p className='text-muted-foreground leading-7'>
                  {t(
                    'Use your gateway address, not the upstream provider address. OpenAI SDK examples include /v1 in the Base URL; avoid adding /v1 twice.'
                  )}
                </p>
                <DocsExample
                  code='https://YOUR_GATEWAY_HOST/v1'
                  label='Base URL'
                />
              </section>
              <section id='quickstart' className='docs-section space-y-4'>
                <h2 className='text-xl font-semibold'>{t('Quick start')}</h2>
                <p className='text-muted-foreground leading-7'>
                  {t(
                    'Replace the host, key, and model placeholders before running these examples on your own machine. This page does not send model requests.'
                  )}
                </p>
                <Tabs defaultValue='curl'>
                  <TabsList aria-label={t('Request examples')}>
                    <TabsTrigger value='curl'>cURL</TabsTrigger>
                    <TabsTrigger value='sdk'>OpenAI SDK · Python</TabsTrigger>
                  </TabsList>
                  <TabsContent value='curl'>
                    <DocsExample code={CURL_EXAMPLE} label='cURL' />
                  </TabsContent>
                  <TabsContent value='sdk'>
                    <DocsExample code={SDK_EXAMPLE} label='Python' />
                  </TabsContent>
                </Tabs>
              </section>
              <section id='streaming' className='docs-section space-y-4'>
                <h2 className='text-xl font-semibold'>{t('Streaming')}</h2>
                <p className='text-muted-foreground leading-7'>
                  {t(
                    'Set stream to true and consume incremental chunks. Streaming support depends on the selected model and API route.'
                  )}
                </p>
                <DocsExample code={STREAM_EXAMPLE} label='Python · stream' />
              </section>
              <section id='models' className='docs-section space-y-4'>
                <h2 className='text-xl font-semibold'>
                  {t('Model selection')}
                </h2>
                <p className='text-muted-foreground leading-7'>
                  {t(
                    'Choose an available model from the Model Square and copy its exact name. Prices, groups, and supported endpoints vary by site configuration.'
                  )}
                </p>
                <Button variant='outline' render={<Link to='/pricing' />}>
                  {t('Model Square')}
                </Button>
              </section>
              <section id='errors' className='docs-section space-y-4'>
                <h2 className='text-xl font-semibold'>{t('Common errors')}</h2>
                <StaticDataTable
                  tableClassName='min-w-[32rem]'
                  data={errors}
                  getRowKey={(row) => row.code}
                  columns={[
                    {
                      id: 'status',
                      header: t('Status'),
                      cell: (row) => row.code,
                      className: 'w-20',
                    },
                    {
                      id: 'description',
                      header: t('Troubleshooting'),
                      cell: (row) => (
                        <p className='text-muted-foreground leading-6 whitespace-normal'>
                          {row.text}
                        </p>
                      ),
                    },
                  ]}
                />
              </section>
              <section id='request-id' className='docs-section space-y-4'>
                <h2 className='text-xl font-semibold'>Request ID</h2>
                <p className='text-muted-foreground leading-7'>
                  {t(
                    'Save X-Nawei-Request-Id from the response headers, together with the time and model name. Search for that Request ID in Usage Logs to find the gateway record.'
                  )}
                </p>
                <p className='text-muted-foreground leading-7'>
                  {t(
                    'An upstream Request ID may also appear in log details when captured. It can be shared with the provider, but it is not guaranteed for every channel or request.'
                  )}
                </p>
                <Button variant='outline' render={<Link to='/usage-logs' />}>
                  {t('Usage Logs')}
                </Button>
              </section>
              <a
                href='https://docs.newapi.ai/'
                target='_blank'
                rel='noopener noreferrer'
                className='text-primary inline-flex items-center gap-2 text-sm'
              >
                {t('Project documentation')}
                <ArrowUpRight aria-hidden='true' className='size-4' />
              </a>
              </div>
            </article>

            {/* Right: on-this-page table of contents */}
            <aside className='sticky top-24 hidden xl:block'>
              <nav
                aria-label={t('On this page')}
                className='flex flex-col gap-1 overflow-y-auto'
              >
                <p className='text-muted-foreground/70 mb-1.5 px-3 text-[11px] font-semibold tracking-wider uppercase'>
                  {t('On this page')}
                </p>
                {sections.map((section) => (
                  <a
                    key={section.id}
                    href={`#${section.id}`}
                    className='text-muted-foreground hover:text-primary focus-visible:outline-ring rounded-lg px-3 py-1.5 text-sm transition-colors'
                  >
                    {section.title}
                  </a>
                ))}
              </nav>
            </aside>
          </div>
        </div>
      </main>
      <Footer />
    </PublicLayout>
  )
}
