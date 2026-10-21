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
import i18next from 'i18next'
import { expect, test } from 'vitest'

import en from '@/i18n/locales/en.json'
import fr from '@/i18n/locales/fr.json'
import ja from '@/i18n/locales/ja.json'
import ru from '@/i18n/locales/ru.json'
import vi from '@/i18n/locales/vi.json'
import zhTW from '@/i18n/locales/zh-TW.json'
import zh from '@/i18n/locales/zh.json'
import { getServerErrorMessage } from '@/lib/server-error-message'

const messages = [
  'Start and end dates are required',
  'Dates must use YYYY-MM-DD format',
  'Start date must not be later than end date',
  'The date range cannot exceed 366 days',
  'Time granularity must be day or month',
  'No data to export for the current filters',
  'Export exceeds 100000 rows. Narrow the filters and try again.',
  'User ID must be a non-negative integer',
  'Page number must be a positive integer',
  'Page size must be an integer between 1 and 100',
  'Failed to query reconciliation bills. Please try again later.',
  'Failed to query reconciliation users. Please try again later.',
  'Failed to query reconciliation models. Please try again later.',
  'Failed to export reconciliation bills. Please try again later.',
]

test('reconciliation errors follow the active language in all seven locales', async () => {
  const resources: Record<string, { translation: Record<string, string> }> = {
    en,
    fr,
    ja,
    ru,
    vi,
    zhCN: zh,
    zhTW,
  }
  const originalLanguage = i18next.language
  const originalResources = structuredClone(i18next.store.data)
  try {
    for (const [language, resource] of Object.entries(resources)) {
      i18next.addResourceBundle(
        language,
        'translation',
        resource.translation,
        true,
        true
      )
      await i18next.changeLanguage(language)
      for (const key of messages) {
        const translation = resource.translation[key]
        expect(translation, `${language}: ${key}`).toBeTruthy()
        if (language !== 'en') expect(translation).not.toBe(key)
        expect(
          getServerErrorMessage({
            success: false,
            message: key,
            message_key: key,
          })
        ).toBe(translation)
      }
    }
  } finally {
    for (const language of Object.keys(resources)) {
      i18next.removeResourceBundle(language, 'translation')
      const original = originalResources[language]?.translation
      if (original) i18next.addResourceBundle(language, 'translation', original)
    }
    await i18next.changeLanguage(originalLanguage)
  }
})
