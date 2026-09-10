/*
Copyright (C) 2025 QuantumNous

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

import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import { useTranslation } from 'react-i18next';
import dayjs from 'dayjs';
import {
  Avatar,
  Button,
  DatePicker,
  Empty,
  Select,
  Tag,
  Typography,
} from '@douyinfe/semi-ui';
import { IconDownload, IconSearch } from '@douyinfe/semi-icons';
import {
  IllustrationNoResult,
  IllustrationNoResultDark,
} from '@douyinfe/semi-illustrations';
import { ReceiptText } from 'lucide-react';
import CardPro from '../../components/common/ui/CardPro';
import CardTable from '../../components/common/ui/CardTable';
import { useIsMobile } from '../../hooks/common/useIsMobile';
import {
  API,
  convertUSDToCurrency,
  createCardProPagination,
  showError,
  showSuccess,
} from '../../helpers';

const DEFAULT_PAGE_SIZE = 20;
const SEARCH_PAGE_SIZE = 20;

const createDateRange = (days) => {
  const end = dayjs().startOf('day');
  return [end.subtract(days - 1, 'day').toDate(), end.toDate()];
};

const createDefaultFilters = () => ({
  dateRange: createDateRange(30),
  userId: undefined,
  modelName: undefined,
  granularity: 'day',
});

const getRangeDays = (dateRange) => {
  if (!Array.isArray(dateRange) || dateRange.length !== 2) return null;
  const start = dayjs(dateRange[0]).startOf('day');
  const end = dayjs(dateRange[1]).startOf('day');
  if (!start.isValid() || !end.isValid()) return null;
  return end.diff(start, 'day') + 1;
};

const toRequestParams = (filters) => ({
  start_date: dayjs(filters.dateRange[0]).format('YYYY-MM-DD'),
  end_date: dayjs(filters.dateRange[1]).format('YYYY-MM-DD'),
  user_id: filters.userId,
  model_name: filters.modelName,
  granularity: filters.granularity,
});

const getInitials = (name) => {
  const normalized = String(name || '').trim();
  return normalized ? normalized.slice(0, 2).toUpperCase() : '?';
};

const getDownloadMessage = async (error, fallback) => {
  const payload = error?.response?.data;
  if (payload instanceof Blob) {
    try {
      const parsed = JSON.parse(await payload.text());
      return parsed.message || fallback;
    } catch (_) {
      return fallback;
    }
  }
  return payload?.message || error?.message || fallback;
};

const Reconciliation = () => {
  const { t } = useTranslation();
  const isMobile = useIsMobile();
  const [filters, setFilters] = useState(createDefaultFilters);
  const [items, setItems] = useState([]);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(DEFAULT_PAGE_SIZE);
  const [loading, setLoading] = useState(false);
  const [exporting, setExporting] = useState(false);
  const [userOptions, setUserOptions] = useState([]);
  const [modelOptions, setModelOptions] = useState([]);
  const [usersLoading, setUsersLoading] = useState(false);
  const [modelsLoading, setModelsLoading] = useState(false);
  const requestSequence = useRef(0);
  const userSearchTimer = useRef(null);
  const modelSearchTimer = useRef(null);

  const validateFilters = useCallback(
    (nextFilters, notify = true) => {
      const days = getRangeDays(nextFilters.dateRange);
      let message = '';
      if (!days) {
        message = t('请选择完整的日期范围');
      } else if (days < 1) {
        message = t('开始日期不能晚于结束日期');
      } else if (days > 366) {
        message = t('日期范围不能超过 366 天');
      }
      if (message && notify) showError(message);
      return !message;
    },
    [t],
  );

  const loadBills = useCallback(
    async (nextFilters, nextPage = 1, nextPageSize = DEFAULT_PAGE_SIZE) => {
      if (!validateFilters(nextFilters)) return;
      const sequence = ++requestSequence.current;
      setLoading(true);
      try {
        const response = await API.get('/api/reconciliation/bills', {
          params: {
            ...toRequestParams(nextFilters),
            p: nextPage,
            page_size: nextPageSize,
          },
          disableDuplicate: true,
        });
        if (!response.data.success) {
          showError(response.data.message || t('获取对账单失败'));
          return;
        }
        if (sequence !== requestSequence.current) return;
        const data = response.data.data || {};
        setItems(data.items || []);
        setTotal(data.total || 0);
        setPage(data.page || nextPage);
        setPageSize(data.page_size || nextPageSize);
      } catch (_) {
        // The shared API client already shows transport and permission errors.
      } finally {
        if (sequence === requestSequence.current) setLoading(false);
      }
    },
    [t, validateFilters],
  );

  useEffect(() => {
    const initialFilters = createDefaultFilters();
    loadBills(initialFilters, 1, DEFAULT_PAGE_SIZE);
  }, [loadBills]);

  useEffect(
    () => () => {
      if (userSearchTimer.current) clearTimeout(userSearchTimer.current);
      if (modelSearchTimer.current) clearTimeout(modelSearchTimer.current);
    },
    [],
  );

  const searchUsers = useCallback((keyword = '') => {
    if (userSearchTimer.current) clearTimeout(userSearchTimer.current);
    userSearchTimer.current = setTimeout(
      async () => {
        setUsersLoading(true);
        try {
          const response = await API.get('/api/reconciliation/options/users', {
            params: {
              keyword: keyword.trim(),
              p: 1,
              page_size: SEARCH_PAGE_SIZE,
            },
          });
          if (response.data.success) {
            setUserOptions(
              (response.data.data?.items || []).map((user) => ({
                value: user.user_id,
                label: user.username
                  ? `${user.username} (#${user.user_id})`
                  : `#${user.user_id}`,
              })),
            );
          }
        } catch (_) {
          setUserOptions([]);
        } finally {
          setUsersLoading(false);
        }
      },
      keyword ? 350 : 0,
    );
  }, []);

  const searchModels = useCallback((keyword = '') => {
    if (modelSearchTimer.current) clearTimeout(modelSearchTimer.current);
    modelSearchTimer.current = setTimeout(
      async () => {
        setModelsLoading(true);
        try {
          const response = await API.get('/api/reconciliation/options/models', {
            params: {
              keyword: keyword.trim(),
              p: 1,
              page_size: SEARCH_PAGE_SIZE,
            },
          });
          if (response.data.success) {
            const names = new Set(
              (response.data.data?.items || [])
                .map((model) => model.model_name)
                .filter(Boolean),
            );
            setModelOptions(
              Array.from(names).map((modelName) => ({
                value: modelName,
                label: modelName,
              })),
            );
          }
        } catch (_) {
          setModelOptions([]);
        } finally {
          setModelsLoading(false);
        }
      },
      keyword ? 350 : 0,
    );
  }, []);

  const applyFilters = (nextFilters) => {
    setFilters(nextFilters);
    if (validateFilters(nextFilters)) {
      setPage(1);
      loadBills(nextFilters, 1, pageSize);
    }
  };

  const applyPreset = (days) => {
    applyFilters({ ...filters, dateRange: createDateRange(days) });
  };

  const resetFilters = () => {
    const nextFilters = createDefaultFilters();
    setUserOptions([]);
    setModelOptions([]);
    applyFilters(nextFilters);
  };

  const handleExport = async () => {
    if (!validateFilters(filters)) return;
    setExporting(true);
    try {
      const response = await API.get('/api/reconciliation/export', {
        params: toRequestParams(filters),
        responseType: 'blob',
        skipErrorHandler: true,
        disableDuplicate: true,
      });
      const contentType = String(response.headers['content-type'] || '');
      if (contentType.includes('application/json')) {
        const payload = JSON.parse(await response.data.text());
        throw new Error(payload.message || t('导出对账单失败'));
      }
      const downloadUrl = URL.createObjectURL(response.data);
      const anchor = document.createElement('a');
      anchor.href = downloadUrl;
      anchor.download = `${t('对账单')}_${dayjs().format('YYYY-MM-DD')}.xlsx`;
      document.body.appendChild(anchor);
      anchor.click();
      anchor.remove();
      URL.revokeObjectURL(downloadUrl);
      showSuccess(t('导出成功'));
    } catch (error) {
      showError(await getDownloadMessage(error, t('导出对账单失败')));
    } finally {
      setExporting(false);
    }
  };

  const activePreset = useMemo(() => {
    for (const days of [7, 30, 90]) {
      const preset = createDateRange(days);
      if (
        dayjs(filters.dateRange?.[0]).isSame(preset[0], 'day') &&
        dayjs(filters.dateRange?.[1]).isSame(preset[1], 'day')
      ) {
        return days;
      }
    }
    return null;
  }, [filters.dateRange]);

  const columns = useMemo(
    () => [
      {
        title: t('用户'),
        dataIndex: 'username',
        width: 260,
        render: (_, record) => {
          const username = record.username || `#${record.user_id}`;
          return (
            <div className='flex items-center gap-2 min-w-[160px]'>
              <Avatar size='small' color='blue'>
                {getInitials(username)}
              </Avatar>
              <div className='min-w-0'>
                <div className='truncate font-medium'>{username}</div>
                <Typography.Text type='tertiary' size='small'>
                  #{record.user_id}
                </Typography.Text>
              </div>
            </div>
          );
        },
      },
      {
        title: t('时间'),
        dataIndex: 'period',
        width: 160,
      },
      {
        title: t('模型'),
        dataIndex: 'model_name',
        render: (modelName) => (
          <Tag color='blue' shape='circle'>
            {modelName || '-'}
          </Tag>
        ),
      },
      {
        title: t('费用'),
        dataIndex: 'amount_usd',
        align: 'right',
        width: 180,
        render: (amount) => (
          <span className='font-mono font-medium tabular-nums'>
            {convertUSDToCurrency(Number(amount || 0), 2)}
          </span>
        ),
      },
    ],
    [t],
  );

  const searchArea = (
    <div className='flex flex-col gap-3'>
      <div className='flex flex-wrap items-center justify-between gap-2'>
        <div className='flex flex-wrap items-center gap-2'>
          {[7, 30, 90].map((days) => (
            <Button
              key={days}
              type={activePreset === days ? 'primary' : 'tertiary'}
              theme={activePreset === days ? 'light' : 'outline'}
              size='small'
              onClick={() => applyPreset(days)}
            >
              {t(`近 ${days} 天`)}
            </Button>
          ))}
        </div>
        <Button
          type='primary'
          icon={<IconDownload />}
          loading={exporting}
          onClick={handleExport}
        >
          {exporting ? t('导出中...') : t('导出')}
        </Button>
      </div>
      <div className='grid grid-cols-1 md:grid-cols-2 xl:grid-cols-4 gap-2'>
        <DatePicker
          type='dateRange'
          value={filters.dateRange}
          format='yyyy-MM-dd'
          placeholder={[t('开始时间'), t('结束时间')]}
          showClear={false}
          onChange={(dateRange) => {
            if (!Array.isArray(dateRange) || dateRange.length !== 2) return;
            applyFilters({ ...filters, dateRange });
          }}
          className='w-full'
          size='small'
        />
        <Select
          value={filters.userId}
          optionList={userOptions}
          placeholder={t('全部用户')}
          prefix={<IconSearch />}
          filter
          remote
          showClear
          loading={usersLoading}
          onFocus={() => userOptions.length === 0 && searchUsers('')}
          onSearch={searchUsers}
          onChange={(userId) =>
            applyFilters({ ...filters, userId: userId || undefined })
          }
          className='w-full'
          size='small'
        />
        <Select
          value={filters.modelName}
          optionList={modelOptions}
          placeholder={t('全部模型')}
          prefix={<IconSearch />}
          filter
          remote
          showClear
          loading={modelsLoading}
          onFocus={() => modelOptions.length === 0 && searchModels('')}
          onSearch={searchModels}
          onChange={(modelName) =>
            applyFilters({ ...filters, modelName: modelName || undefined })
          }
          className='w-full'
          size='small'
        />
        <Select
          value={filters.granularity}
          optionList={[
            { value: 'day', label: t('按日') },
            { value: 'month', label: t('按月') },
          ]}
          onChange={(granularity) => applyFilters({ ...filters, granularity })}
          className='w-full'
          size='small'
        />
      </div>
      <div className='flex justify-end gap-2'>
        <Button
          type='tertiary'
          onClick={() => loadBills(filters, 1, pageSize)}
          loading={loading}
          size='small'
        >
          {t('查询')}
        </Button>
        <Button type='tertiary' onClick={resetFilters} size='small'>
          {t('重置')}
        </Button>
      </div>
    </div>
  );

  return (
    <div className='mt-[60px] px-2'>
      <CardPro
        type='type1'
        descriptionArea={
          <div className='flex items-center gap-3'>
            <Avatar size='small' color='blue'>
              <ReceiptText size={16} />
            </Avatar>
            <div>
              <Typography.Title heading={5}>{t('对账单')}</Typography.Title>
              <Typography.Text type='tertiary' size='small'>
                {t('只读汇总现有消费日志，不影响原有统计。')}
              </Typography.Text>
            </div>
          </div>
        }
        actionsArea={searchArea}
        paginationArea={createCardProPagination({
          currentPage: page,
          pageSize,
          total,
          onPageChange: (nextPage) => {
            setPage(nextPage);
            loadBills(filters, nextPage, pageSize);
          },
          onPageSizeChange: (nextPageSize) => {
            setPage(1);
            setPageSize(nextPageSize);
            loadBills(filters, 1, nextPageSize);
          },
          isMobile,
          t,
        })}
        t={t}
      >
        <CardTable
          columns={columns}
          dataSource={items}
          rowKey={(record) =>
            `${record.user_id}-${record.period}-${record.model_name}`
          }
          loading={loading}
          pagination={false}
          hidePagination
          tableLayout='fixed'
          className='w-full rounded-xl overflow-hidden'
          style={{ width: '100%' }}
          size='small'
          empty={
            <Empty
              image={
                <IllustrationNoResult style={{ width: 150, height: 150 }} />
              }
              darkModeImage={
                <IllustrationNoResultDark style={{ width: 150, height: 150 }} />
              }
              title={t('暂无对账数据')}
              description={t('当前筛选条件下没有消费记录')}
              style={{ padding: 30 }}
            />
          }
        />
      </CardPro>
    </div>
  );
};

export default Reconciliation;
