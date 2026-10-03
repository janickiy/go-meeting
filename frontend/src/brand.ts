/** Название и подпись бренда задаются один раз при сборке, без удалённого HTML или URL. */
export const PRODUCT_NAME =
  (import.meta.env.VITE_PRODUCT_NAME as string | undefined)
    ?.trim()
    .slice(0, 40) || "Meetrix";
export const PRODUCT_TAGLINE = "Встречи. Идеи. Результаты.";
