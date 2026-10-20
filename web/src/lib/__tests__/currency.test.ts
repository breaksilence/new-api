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
import { afterEach, expect, test } from 'vitest'

import { toIntlLocale } from '@/i18n/languages'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { formatBillingCurrencyFromUSD, getCurrencyLabel } from '../currency'

afterEach(() => {
  useSystemConfigStore
    .getState()
    .setConfig({ currency: { ...DEFAULT_CURRENCY_CONFIG } })
})

test.each([
  ['zhCN', '¥12.00'],
  ['zhTW', '¥12.00'],
  ['en', '¥12.00'],
  ['ja', '￥12.00'],
  ['fr', '12,00 ¥'],
  ['ru', '12,00 ¥'],
  ['vi', '12,00 ¥'],
])(
  'retains two fee decimals and the CNY display convention in %s',
  (language, expected) => {
    useSystemConfigStore.getState().setConfig({
      currency: {
        ...DEFAULT_CURRENCY_CONFIG,
        quotaDisplayType: 'USD',
        usdExchangeRate: 7,
      },
    })
    const formatted = formatBillingCurrencyFromUSD(12, {
      locale: toIntlLocale(language),
      minimumFractionDigits: 2,
      abbreviate: false,
    })
    expect(formatted.replaceAll(/\s/g, ' ')).toBe(expected)
    expect(getCurrencyLabel()).toBe('CNY')
  }
)

test('keeps explicit CNY/custom exchange rates and compact output without changing underlying values', () => {
  useSystemConfigStore.getState().setConfig({
    currency: {
      ...DEFAULT_CURRENCY_CONFIG,
      quotaDisplayType: 'CNY',
      usdExchangeRate: 7,
    },
  })
  expect(
    formatBillingCurrencyFromUSD(2, {
      locale: 'en',
      minimumFractionDigits: 2,
      abbreviate: false,
    })
  ).toBe('¥14.00')
  expect(
    formatBillingCurrencyFromUSD(2, {
      locale: 'en',
      minimumFractionDigits: 2,
      abbreviate: false,
      showSymbol: false,
    })
  ).toBe('14.00')
  expect(
    formatBillingCurrencyFromUSD(1000, {
      locale: 'en',
      minimumFractionDigits: 2,
      compact: true,
      abbreviate: false,
    })
  ).toBe('¥7.0K')
  useSystemConfigStore.getState().setConfig({
    currency: {
      ...DEFAULT_CURRENCY_CONFIG,
      quotaDisplayType: 'CUSTOM',
      customCurrencySymbol: '€',
      customCurrencyExchangeRate: 0.5,
    },
  })
  expect(
    formatBillingCurrencyFromUSD(2, {
      locale: 'en',
      minimumFractionDigits: 2,
      abbreviate: false,
    })
  ).toBe('€ 1.00')
  expect(() =>
    formatBillingCurrencyFromUSD(2, {
      locale: toIntlLocale('not_a_locale'),
      minimumFractionDigits: 2,
    })
  ).not.toThrow()
})
