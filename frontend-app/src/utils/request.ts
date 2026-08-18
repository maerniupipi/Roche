// src/utils/request.js
import axios from "axios";
import { generateRandomString, MAX_FILE_SIZE_MB } from "./index";
import i18n from '@/i18n'
import { getApiBaseUrl } from './api-base';
import { getLoginPath } from './auth-redirect';

const t = (key: string) => i18n.global.t(key)

// API基础URL
const BASE_URL = getApiBaseUrl();


// 创建Axios实例
const instance = axios.create({
  baseURL: BASE_URL, // 使用配置的API基础URL
  timeout: 30000, // 请求超时时间
  withCredentials: true,
  headers: {
    "Content-Type": "application/json",
    "X-Request-ID": `${generateRandomString(12)}`,
  },
});

// 获取当前用户语言（用于 Accept-Language header）
function getCurrentLanguage(): string {
  return i18n.global.locale?.value || localStorage.getItem('locale') || 'zh-CN'
}


instance.interceptors.request.use(
  (config) => {
    const token = localStorage.getItem('roche_kap_token');
    if (token) {
      config.headers["Authorization"] = `Bearer ${token}`;
    }

    // 添加用户语言偏好
    config.headers["Accept-Language"] = getCurrentLanguage();

    // 添加知识域作用域请求头：只要 setSelectedKnowledgeDomain 写过当前知识域，
    // 每个请求都要附 X-KnowledgeDomain-ID。早期版本会 short-circuit
    // "selectedKnowledgeDomainId === defaultKnowledgeDomainId 时不附"以减少 header 体积，
    // 但这条优化会被任何把 roche_kap_knowledgeDomain 写成当前知识域的代码（
    // 回调、UserMenu loadUserInfo、router hydrate）触发，导致后续请求
    // 静默丢失 header，前端"切换了"但实际仍跑在默认知识域里——把"切
    // 换之后只有第一批请求带 X-KnowledgeDomain-ID"调成永久状态。
    // 后端仍会校验用户是否有权访问 header 指定的知识域。
    config.headers["X-Request-ID"] = `${generateRandomString(12)}`;
    return config;
  },
  (error) => {
    return Promise.reject(error);
  }
);

// Token刷新标志，防止多个请求同时刷新token
let isRefreshing = false;
let failedQueue: Array<{ resolve: Function; reject: Function }> = [];

const PUBLIC_AUTH_PATHS = [
  '/auth/auto-setup',
  '/auth/login',
  '/auth/register',
  '/auth/registration/',
  '/auth/saml/',
];

function isPublicAuthRequest(url?: string): boolean {
  if (!url) return false;
  return PUBLIC_AUTH_PATHS.some(p => url.includes(p));
}

// 处理队列中的请求
const processQueue = (error: any, token: string | null = null) => {
  failedQueue.forEach(({ resolve, reject }) => {
    if (error) {
      reject(error);
    } else {
      resolve(token);
    }
  });

  failedQueue = [];
};

function redirectToLogin() {
  if (typeof window === 'undefined') return;
  const loginPath = getLoginPath(import.meta.env.BASE_URL);
  if (window.location.pathname === loginPath) return;
  window.location.href = loginPath;
}

instance.interceptors.response.use(
  (response) => {
    // 根据业务状态码处理逻辑
    const { status, data } = response;
    if (status >= 200 && status < 300) {
      return data;
    } else {
      return Promise.reject(data);
    }
  },
  async (error: any) => {
    const originalRequest = error.config;

    if (!error.response) {
      return Promise.reject({ message: t('error.networkError') });
    }

    // 公开认证接口的 401/403 不走 refresh 逻辑，直接返回错误。
    if ((error.response.status === 401 || error.response.status === 403) && isPublicAuthRequest(originalRequest?.url)) {
      const { status, data } = error.response;
      const msg = typeof data === 'object'
        ? (typeof data?.error === 'string' ? data.error : (data?.error?.message || data?.message))
        : data;
      return Promise.reject({ status, message: msg || t('error.invalidCredentials') });
    }

    // 如果是401错误且不是刷新token的请求，尝试刷新token
    if (error.response.status === 401 && !originalRequest._retry && !originalRequest.url?.includes('/auth/refresh')) {
      if (isRefreshing) {
        // 如果正在刷新token，将请求加入队列
        return new Promise((resolve, reject) => {
          failedQueue.push({ resolve, reject });
        }).then(token => {
          originalRequest.headers['Authorization'] = 'Bearer ' + token;
          return instance(originalRequest);
        }).catch(err => {
          return Promise.reject(err);
        });
      }

      originalRequest._retry = true;
      isRefreshing = true;

      try {
        // The browser sends the Auth Service HttpOnly refresh cookie.
        const { refreshToken: refreshTokenAPI } = await import('../api/auth/index');
        const response = await refreshTokenAPI();

        if (response.success && response.data) {
          const { token } = response.data;
          localStorage.setItem('roche_kap_token', token);
          originalRequest.headers['Authorization'] = 'Bearer ' + token;
          processQueue(null, token);
          return instance(originalRequest);
        }
        throw new Error(response.message || t('error.tokenRefreshFailed'));
      } catch (refreshError) {
        localStorage.removeItem('roche_kap_token');
        localStorage.removeItem('roche_kap_refresh_token');
        localStorage.removeItem('roche_kap_user');
        processQueue(refreshError, null);
        redirectToLogin();
        return Promise.reject(refreshError);
      } finally {
        isRefreshing = false;
      }
    }

    // 处理 Nginx 413 Request Entity Too Large
    if (error.response.status === 413) {
      return Promise.reject({
        status: 413,
        message: i18n.global.t('error.fileSizeExceeded', { size: MAX_FILE_SIZE_MB }),
        success: false
      });
    }

    const { status, data } = error.response;
    // 将HTTP状态码一并抛出，方便上层判断401等场景
    // 后端返回格式: { success: false, error: { code, message, details } }
    // 提取 error.message 作为顶层 message，方便前端使用 error?.message 获取
    let errorMessage: string | undefined;
    if (typeof data === 'object') {
      if (typeof data?.error === 'string') {
        errorMessage = data.error;
      } else if (data?.error?.message) {
        errorMessage = data.error.message;
      } else {
        errorMessage = data?.message;
      }
    } else if (typeof data === 'string') {
      errorMessage = data;
    }
    return Promise.reject({
      status,
      message: errorMessage,
      ...(typeof data === 'object' ? data : {})
    });
  }
);

export function get<T = any>(url: string, config?: any): Promise<T> {
  return instance.get<T>(url, config) as unknown as Promise<T>;
}

export async function getDown(url: string): Promise<Blob> {
  const res = await instance.get<Blob>(url, {
    responseType: "blob",
  }) as unknown as Blob;
  return res
}

export function postUpload(
  url: string,
  data = {},
  onUploadProgress?: (progressEvent: any) => void,
  config: any = {},
): Promise<any> {
  return instance.post(url, data, {
    ...config,
    headers: {
      "Content-Type": "multipart/form-data",
      "X-Request-ID": `${generateRandomString(12)}`,
      ...(config.headers || {}),
    },
    onUploadProgress: onUploadProgress || config.onUploadProgress,
  }) as unknown as Promise<any>;
}

export function postChat<T = any>(url: string, data = {}): Promise<T> {
  return instance.post(url, data, {
    headers: {
      "Content-Type": "text/event-stream;charset=utf-8",
      "X-Request-ID": `${generateRandomString(12)}`,
    },
  }) as unknown as Promise<T>;
}

export function post<T = any>(url: string, data = {}, config?: any): Promise<T> {
  return instance.post<T>(url, data, config) as unknown as Promise<T>;
}

export function put<T = any>(url: string, data = {}, config?: any): Promise<T> {
  return instance.put<T>(url, data, config) as unknown as Promise<T>;
}

export function del<T = any>(url: string, data?: any): Promise<T> {
  return instance.delete<T>(url, { data }) as unknown as Promise<T>;
}
