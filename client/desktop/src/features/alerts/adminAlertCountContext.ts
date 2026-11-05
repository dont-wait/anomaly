import { createContext, useContext } from "react";

export const AdminAlertCountContext = createContext(0);

export function useAdminAlertCount() {
  return useContext(AdminAlertCountContext);
}
