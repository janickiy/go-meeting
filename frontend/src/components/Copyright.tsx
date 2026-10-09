/** Показывает единый копирайт с безопасной ссылкой на сайт автора.
 * @return Точный текст копирайта; сайт открывается в отдельной вкладке без доступа к исходной странице.
 */
export function Copyright() {
  return (
    <p className="copyright">
      © 2026{" "}
      <a href="https://janickiy.com/" target="_blank" rel="noopener noreferrer">
        Яницкий Александр
      </a>
      . Все права защищены.
    </p>
  );
}
