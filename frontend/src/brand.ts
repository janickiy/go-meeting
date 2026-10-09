/** Название и подпись бренда задаются один раз при сборке, без удалённого HTML или URL. */
const configuredProductName = (
  import.meta.env.VITE_PRODUCT_NAME as string | undefined
)
  ?.trim()
  .slice(0, 40);
// Старое публичное build-time имя не должно возвращать прежний бренд после обновления.
export const PRODUCT_NAME =
  !configuredProductName || ["Meetrix", "Meet"].includes(configuredProductName)
    ? "MeetSpace"
    : configuredProductName;
export const PRODUCT_TAGLINE =
  "Встречи, чаты и совместная работа в одном месте.";
export const PRODUCT_DESCRIPTION = `${PRODUCT_NAME} — единое пространство для встреч и общения.`;
