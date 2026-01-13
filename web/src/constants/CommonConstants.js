export const ITEMS_PER_PAGE = 10; // this value must keep same as the one defined in backend!
export const PAGE_SIZE_OPTIONS = [10, 30, 50, 100];

// 页面分页大小本地存储键
const PAGE_SIZE_STORAGE_KEY = 'user_page_sizes';
// 表格排序状态本地存储键
const TABLE_SORT_STORAGE_KEY = 'user_table_sort';

/**
 * 从localStorage获取页面大小
 * @param {string} pageKey - 页面唯一标识
 * @param {number} defaultSize - 默认页面大小
 * @returns {number} - 页面大小
 */
export const getPageSize = (pageKey, defaultSize = ITEMS_PER_PAGE) => {
  try {
    const pageSizesStr = localStorage.getItem(PAGE_SIZE_STORAGE_KEY);
    if (pageSizesStr) {
      const pageSizes = JSON.parse(pageSizesStr);
      if (pageSizes[pageKey] !== undefined) {
        return parseInt(pageSizes[pageKey], 10);
      }
    }
    return defaultSize;
  } catch (error) {
    console.error('Error while getting page size:', error);
    return defaultSize;
  }
};

/**
 * 保存页面大小到localStorage
 * @param {string} pageKey - 页面唯一标识
 * @param {number} size - 页面大小
 */
export const savePageSize = (pageKey, size) => {
  try {
    let pageSizes = {};
    const pageSizesStr = localStorage.getItem(PAGE_SIZE_STORAGE_KEY);
    if (pageSizesStr) {
      try {
        pageSizes = JSON.parse(pageSizesStr);
      } catch (parseError) {
        console.error('Failed to parse page size data from localStorage, resetting data:', parseError);
      }
    }
    pageSizes[pageKey] = size;
    localStorage.setItem(PAGE_SIZE_STORAGE_KEY, JSON.stringify(pageSizes));
  } catch (error) {
    console.error('Error while saving page size:', error);
    try {
      const singlePageData = {};
      singlePageData[pageKey] = size;
      localStorage.setItem(PAGE_SIZE_STORAGE_KEY, JSON.stringify(singlePageData));
    } catch (fallbackError) {
      console.error('Failed to save single page size settings:', fallbackError);
    }
  }
};

/**
 * 从localStorage获取表格排序状态
 * @param {string} tableKey - 表格唯一标识
 * @param {string} defaultOrder - 默认排序方向
 * @param {string} defaultOrderBy - 默认排序字段
 * @returns {{order: string, orderBy: string}} - 排序状态
 */
export const getTableSort = (tableKey, defaultOrder = 'desc', defaultOrderBy = 'id') => {
  try {
    const sortDataStr = localStorage.getItem(TABLE_SORT_STORAGE_KEY);
    if (sortDataStr) {
      const sortData = JSON.parse(sortDataStr);
      if (sortData[tableKey]) {
        return {
          order: sortData[tableKey].order || defaultOrder,
          orderBy: sortData[tableKey].orderBy || defaultOrderBy
        };
      }
    }
    return { order: defaultOrder, orderBy: defaultOrderBy };
  } catch (error) {
    console.error('Error while getting table sort:', error);
    return { order: defaultOrder, orderBy: defaultOrderBy };
  }
};

/**
 * 保存表格排序状态到localStorage
 * @param {string} tableKey - 表格唯一标识
 * @param {string} order - 排序方向
 * @param {string} orderBy - 排序字段
 */
export const saveTableSort = (tableKey, order, orderBy) => {
  try {
    let sortData = {};
    const sortDataStr = localStorage.getItem(TABLE_SORT_STORAGE_KEY);
    if (sortDataStr) {
      try {
        sortData = JSON.parse(sortDataStr);
      } catch (parseError) {
        console.error('Failed to parse table sort data from localStorage, resetting data:', parseError);
      }
    }
    sortData[tableKey] = { order, orderBy };
    localStorage.setItem(TABLE_SORT_STORAGE_KEY, JSON.stringify(sortData));
  } catch (error) {
    console.error('Error while saving table sort:', error);
  }
};
