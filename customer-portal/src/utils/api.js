import axios from 'axios';

const STORAGE_KEY = 'portal_user';

export function getPortalUser() {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return null;
    return JSON.parse(raw);
  } catch {
    return null;
  }
}

export function setPortalUser(user) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(user));
}

export function clearPortalUser() {
  localStorage.removeItem(STORAGE_KEY);
}

const api = axios.create({
  baseURL: '',
  headers: {
    'Content-Type': 'application/json',
  },
});

// Request interceptor: attach New-Api-User header from stored user data
api.interceptors.request.use((config) => {
  const user = getPortalUser();
  if (user && user.id) {
    config.headers['New-Api-User'] = String(user.id);
  }
  return config;
});

// Response interceptor: on 401, clear user data and redirect to login
api.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error.response && error.response.status === 401) {
      clearPortalUser();
      // Avoid redirect loop if already on login page
      if (!window.location.pathname.includes('/portal/login')) {
        window.location.href = '/portal/login';
      }
    }
    return Promise.reject(error);
  },
);

export default api;
