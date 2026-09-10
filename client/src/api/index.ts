export { ApiError, requestJson } from "./http";
export type { HttpMethod, RequestJsonOptions } from "./http";
export {
  CCCD_LENGTH,
  assertValidLoginInput,
  getCurrentUser,
  isValidCccd,
  login,
  normalizeCccd,
  toLoginError,
} from "./auth";
export type {
  AuthUser,
  LoginCredentials,
  LoginInput,
  LoginResponse,
} from "./auth";
export {
  AUTH_TOKEN_STORAGE_KEY,
  createLocalStorageAuthTokenStore,
  defaultAuthTokenStore,
} from "./session";
export type { AuthTokenStore } from "./session";
