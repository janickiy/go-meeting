/** Показывает компактное число непрочитанных сообщений в чате или общей навигации.
 * Нулевой счётчик не занимает места; полное число доступно экранному диктору.
 * @args count — серверное число непрочитанных личных и/или групповых сообщений.
 * @return числовой индикатор по макету либо отсутствие элемента при нуле.
 */
export function UnreadMessageCount({ count }: { count: number }) {
  if (count <= 0) return null;
  return (
    <span
      className="message-unread-count"
      aria-label={`${count} непрочитанных сообщений`}
    >
      {count}
    </span>
  );
}
