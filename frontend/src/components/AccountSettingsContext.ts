import { createContext, useContext } from "react";

/** Открывает общее окно настроек поверх текущей страницы и возвращает фокус инициатору. */
export const AccountSettingsContext = createContext<
  ((opener: HTMLElement) => void) | undefined
>(undefined);

export function useAccountSettings() {
  return useContext(AccountSettingsContext);
}
