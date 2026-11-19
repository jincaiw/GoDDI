/**
 * Response envelope helpers for the Goddi API.
 * Kept separate from the axios client so components and tests can use them
 * without importing transport state.
 */
import type { PaginatedApiResponse } from '@/service/api/goddi/client';

export interface ListResult<T> {
  data: T[];
  meta: PaginatedApiResponse<T>['meta'];
}

export function successCode(code: unknown): boolean {
  return code === undefined || code === 0;
}

export function normalizeList<T>(body: unknown): ListResult<T> {
  const value = body as Record<string, unknown>;
  if (value.data !== undefined && value.meta !== undefined) {
    return { data: value.data as T[], meta: value.meta as PaginatedApiResponse<T>['meta'] };
  }
  if (Array.isArray(value.data)) {
    return { data: value.data as T[], meta: { total: value.data.length, page: 1, page_size: value.data.length, pages: 1 } };
  }
  return { data: [], meta: { total: 0, page: 1, page_size: 20, pages: 0 } };
}