import { lazy, Suspense } from "react";
import { Loading, Modal } from "./ui";
import "./account-settings-modal.css";

const SettingsContent = lazy(() =>
  import("../pages/AccountPages").then((module) => ({
    default: module.SettingsPage,
  })),
);

/** Открывает настройки поверх текущей страницы, не размонтируя её и активные соединения.
 * @args onClose — закрытие диалога; returnFocus — поиск доступного элемента, открывшего окно.
 * @return Модальное окно с доступными вкладками настроек аккаунта.
 */
export function AccountSettingsModal({
  onClose,
  returnFocus,
}: {
  onClose: () => void;
  returnFocus?: () => HTMLElement | null;
}) {
  return (
    <Modal
      title="Настройки аккаунта"
      onClose={onClose}
      returnFocus={returnFocus}
      wide
      className="account-settings-modal"
    >
      <Suspense fallback={<Loading />}>
        <SettingsContent />
      </Suspense>
    </Modal>
  );
}
