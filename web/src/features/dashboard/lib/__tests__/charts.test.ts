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
import { afterEach, beforeEach, describe, expect, test } from 'vitest'

import type { QuotaDataItem } from '@/features/dashboard/types'
import type { TimeGranularity } from '@/lib/time'
import {
  DEFAULT_CURRENCY_CONFIG,
  useSystemConfigStore,
} from '@/stores/system-config-store'

import { processChartData, processUserChartData } from '../charts'

describe('user consumption amount display', () => {
  const originalConfig = useSystemConfigStore.getState().config
  let originalLanguage: string
  beforeEach(() => {
    originalLanguage = i18next.language
    useSystemConfigStore
      .getState()
      .setConfig({ currency: { ...DEFAULT_CURRENCY_CONFIG } })
  })
  afterEach(async () => {
    useSystemConfigStore.setState({ config: originalConfig })
    await i18next.changeLanguage(originalLanguage)
  })

  test.each([
    { quota: 32, expected: '<¥0.01' },
    { quota: 0, expected: '¥0.00' },
    { quota: 5000, expected: '¥0.01' },
    { quota: 40000, expected: '¥0.08' },
  ])(
    'formats quota $quota consistently in labels and both tooltip modes',
    ({ quota, expected }) => {
      const datum = { User: 'alice', rawQuota: quota }
      const result = processUserChartData([
        {
          username: 'alice',
          model_name: 'model-a',
          created_at: 1767225600,
          quota,
          count: 1,
        },
      ])
      const rank = result.spec_user_rank
      expect(rank.label.formatMethod(quota)).toBe(expected)
      expect(rank.tooltip.mark.content[0].value(datum)).toBe(expected)
      expect(rank.tooltip.dimension.content[0].value(datum)).toBe(expected)
      expect(rank.title.subtext).toBe(`Total: ${expected}`)
    }
  )
})

describe('dashboard display settings', () => {
  const originalConfig = useSystemConfigStore.getState().config
  let originalLanguage: string
  beforeEach(() => {
    originalLanguage = i18next.language
    useSystemConfigStore
      .getState()
      .setConfig({ currency: { ...DEFAULT_CURRENCY_CONFIG } })
  })
  afterEach(async () => {
    useSystemConfigStore.setState({ config: originalConfig })
    await i18next.changeLanguage(originalLanguage)
  })
  test.each(['zhCN', 'zhTW', 'en', 'fr', 'ru', 'ja', 'vi', 'invalid_locale'])(
    'preserves the configured monetary unit with interface language %s',
    async (language) => {
      await i18next.changeLanguage(language)
      const result = processUserChartData([
        {
          username: 'alice',
          model_name: 'model-a',
          created_at: 1767225600,
          quota: 32,
          count: 1,
        },
      ])
      expect(result.spec_user_rank.label.formatMethod(32)).toBe('<¥0.01')
    }
  )
  test.each([
    { type: 'CNY' as const, quota: 500000, expected: '¥7.00' },
    { type: 'CUSTOM' as const, quota: 500000, expected: '¤ 2.00' },
    { type: 'TOKENS' as const, quota: 32, expected: '32' },
  ])(
    'preserves $type currency conversion or raw-unit display',
    ({ type, quota, expected }) => {
      useSystemConfigStore.getState().setConfig({
        currency: {
          ...DEFAULT_CURRENCY_CONFIG,
          quotaDisplayType: type,
          usdExchangeRate: 7,
          customCurrencyExchangeRate: 2,
        },
      })
      const result = processUserChartData([
        {
          username: 'alice',
          model_name: 'model-a',
          created_at: 1767225600,
          quota,
          count: 1,
        },
      ])
      expect(result.spec_user_rank.label.formatMethod(quota)).toBe(expected)
    }
  )
})

const cases: Array<{
  granularity: TimeGranularity
  start: string
  interval: number
  labels: string[]
}> = [
  {
    granularity: 'hour',
    start: '2025-12-31T20:00:00',
    interval: 3600,
    labels: [
      '12-31 20:00',
      '12-31 21:00',
      '12-31 22:00',
      '12-31 23:00',
      '01-01 00:00',
      '01-01 01:00',
      '01-01 02:00',
      '01-01 03:00',
    ],
  },
  {
    granularity: 'day',
    start: '2025-12-27T12:00:00',
    interval: 86400,
    labels: [
      '12-27',
      '12-28',
      '12-29',
      '12-30',
      '12-31',
      '01-01',
      '01-02',
      '01-03',
    ],
  },
  {
    granularity: 'week',
    start: '2025-11-29T12:00:00',
    interval: 604800,
    labels: [
      '11-24 - 11-30',
      '12-01 - 12-07',
      '12-08 - 12-14',
      '12-15 - 12-21',
      '12-22 - 12-28',
      '12-29 - 01-04',
      '01-05 - 01-11',
      '01-12 - 01-18',
    ],
  },
]

function unorderedUsage(start: string, interval: number): QuotaDataItem[] {
  const first = new Date(start).getTime() / 1000
  const rows = Array.from({ length: 8 }, (_, i) => ({
    created_at: first + i * interval,
    model_name: 'model-a',
    username: 'alice',
    quota: (i + 1) * 500000,
    count: i + 1,
  }))
  // Same hour, or a second hourly record within the same day/week label.
  const duplicate = {
    ...rows[0],
    created_at: first + (interval === 3600 ? 0 : 3600),
    quota: 5000000,
    count: 10,
  }
  return [
    rows[7],
    rows[2],
    duplicate,
    rows[5],
    rows[0],
    rows[6],
    rows[1],
    rows[4],
    rows[3],
  ]
}

describe.each(cases)('dashboard $granularity chart chronology', (scenario) => {
  test('orders model buckets across New Year while aggregating repeated labels', () => {
    const data = unorderedUsage(scenario.start, scenario.interval)
    const original = structuredClone(data)
    const result = processChartData(data, scenario.granularity)

    for (const key of ['spec_line', 'spec_area', 'spec_model_line'] as const) {
      const values: Array<{ Time: string; rawQuota: number; Count: number }> =
        result[key].data[0].values
      expect(values.map((row) => row.Time)).toEqual(scenario.labels)
      expect(
        values.map((row) =>
          key === 'spec_model_line' ? row.Count : row.rawQuota / 500000
        )
      ).toEqual([11, 2, 3, 4, 5, 6, 7, 8])
    }
    expect(data).toEqual(original)
  })

  test('orders user buckets across New Year while aggregating repeated labels', () => {
    const data = unorderedUsage(scenario.start, scenario.interval)
    const original = structuredClone(data)
    const result = processUserChartData(data, scenario.granularity)
    const values: Array<{ Time: string; rawQuota: number }> =
      result.spec_user_trend.data[0].values

    expect(values.map((row) => row.Time)).toEqual(scenario.labels)
    expect(values.map((row) => row.rawQuota / 500000)).toEqual([
      11, 2, 3, 4, 5, 6, 7, 8,
    ])
    expect(data).toEqual(original)
  })

  test('pads the selected interval in chronological order even with one populated bucket', () => {
    const data = [unorderedUsage(scenario.start, scenario.interval)[0]]
    const start = new Date(scenario.start).getTime() / 1000
    const result = processChartData(
      data,
      scenario.granularity,
      undefined,
      undefined,
      start + scenario.interval,
      start + 7 * scenario.interval
    )

    for (const key of ['spec_line', 'spec_area', 'spec_model_line'] as const) {
      const values: Array<{ Time: string; rawQuota: number; Count: number }> =
        result[key].data[0].values
      expect(values.map((row) => row.Time)).toEqual(scenario.labels.slice(1))
      expect(
        values.map((row) =>
          key === 'spec_model_line' ? row.Count : row.rawQuota / 500000
        )
      ).toEqual([0, 0, 0, 0, 0, 0, 8])
    }
  })
})

describe('dashboard partial time buckets', () => {
  test.each([
    {
      granularity: 'hour' as const,
      start: '2026-09-01T15:45:00',
      end: '2026-09-01T16:10:00',
      lastRecord: '2026-09-01T16:05:00',
      labels: ['09-01 15:00', '09-01 16:00'],
    },
    {
      granularity: 'day' as const,
      start: '2026-09-01T15:00:00',
      end: '2026-09-02T10:00:00',
      lastRecord: '2026-09-02T09:00:00',
      labels: ['09-01', '09-02'],
    },
    {
      granularity: 'week' as const,
      start: '2026-08-29T10:00:00',
      end: '2026-09-28T10:00:00',
      lastRecord: '2026-09-28T09:00:00',
      labels: [
        '08-24 - 08-30',
        '08-31 - 09-06',
        '09-07 - 09-13',
        '09-14 - 09-20',
        '09-21 - 09-27',
        '09-28 - 10-04',
      ],
    },
  ])(
    'includes the final partial $granularity bucket in every model chart',
    (scenario) => {
      const result = processChartData(
        [
          {
            created_at: new Date(scenario.lastRecord).getTime() / 1000,
            model_name: 'model-a',
            username: 'alice',
            quota: 1000000,
            count: 2,
          },
        ],
        scenario.granularity,
        undefined,
        undefined,
        new Date(scenario.start).getTime() / 1000,
        new Date(scenario.end).getTime() / 1000
      )

      for (const key of [
        'spec_line',
        'spec_area',
        'spec_model_line',
      ] as const) {
        const values: Array<{ Time: string; rawQuota: number; Count: number }> =
          result[key].data[0].values
        expect(values.map((row) => row.Time)).toEqual(scenario.labels)
        expect(values.at(-1)).toMatchObject(
          key === 'spec_model_line' ? { Count: 2 } : { rawQuota: 1000000 }
        )
      }
      expect(result.spec_pie.data[0].values).toEqual([
        { type: 'model-a', value: 2 },
      ])
    }
  )

  // Also run this suite with TZ=America/New_York to exercise DST boundaries.
  test.each([
    ['2026-03-07T15:00:00', '2026-03-09T00:30:00', ['03-07', '03-08', '03-09']],
    ['2026-10-31T15:00:00', '2026-11-02T00:30:00', ['10-31', '11-01', '11-02']],
  ])(
    'uses calendar days across clock changes from %s',
    (start, end, labels) => {
      const timestamp = new Date(end).getTime() / 1000
      const result = processChartData(
        [
          {
            created_at: timestamp,
            model_name: 'model-a',
            quota: 500000,
            count: 1,
          },
        ],
        'day',
        undefined,
        undefined,
        new Date(start).getTime() / 1000,
        timestamp
      )
      expect(
        result.spec_line.data[0].values.map((row: { Time: string }) => row.Time)
      ).toEqual(labels)
      expect(result.spec_line.data[0].values.at(-1)).toMatchObject({
        rawQuota: 500000,
      })
    }
  )

  test('does not duplicate the aggregated local hour when clocks move back', () => {
    const result = processChartData(
      [
        {
          created_at: new Date('2026-11-01T01:30:00').getTime() / 1000,
          model_name: 'model-a',
          quota: 500000,
          count: 1,
        },
      ],
      'hour',
      undefined,
      undefined,
      new Date('2026-11-01T00:00:00').getTime() / 1000,
      new Date('2026-11-01T03:00:00').getTime() / 1000
    )
    const values: Array<{ Time: string; Count: number }> =
      result.spec_model_line.data[0].values
    expect(values.map((row) => row.Time)).toEqual([
      '11-01 00:00',
      '11-01 01:00',
      '11-01 02:00',
      '11-01 03:00',
    ])
    expect(values.reduce((sum, row) => sum + row.Count, 0)).toBe(1)
  })
})
