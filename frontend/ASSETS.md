# Визуальные материалы Meet

## Главная иллюстрация

`public/media/meet-laptop.png` — иллюстрация ноутбука для приветственного экрана,
созданная встроенным инструментом генерации изображений Codex (`imagegen`).
Она создана по визуальному референсу пользователя, а не скопирована из стороннего сайта.
Исходный результат сохранён также в каталоге `~/.codex/generated_images/` текущего чата.
Растровый файл используется только как иллюстрация: изображённые видеозвонки не
означают наличие реализованной видеосвязи в текущем этапе продукта.

Финальный промпт генерации:

```text
Use case: product-mockup
Asset type: hero illustration for an implemented video-conferencing website, not a full page mockup.
Input images: Image 1 is a style and composition reference only; specifically reference the laptop photograph on its first panel, do not reproduce the collage or its text.
Primary request: Create a premium photorealistic silver laptop, slightly angled and open, showing a dark video-call interface with exactly four friendly adult coworkers in a 2x2 video grid, two men and two women. Natural home-office backgrounds inside each tile, one person waving. Small dark round call-control icons and a red end-call circle at the bottom of the laptop screen. The laptop rests on a very light cool-white desk, with just one softly blurred small green plant behind it as in the reference. A soft natural contact shadow.
Composition/framing: Wide landscape asset, laptop fills most of the frame, complete laptop silhouette visible, front three-quarter view. This will be positioned on the right half of a landing page. No page headline or navigation.
Lighting/mood: Soft daylight, clean airy blue-white SaaS website palette, realistic material and skin texture, restrained professional polish.
Constraints: No brands, no logos, no watermark, no readable text anywhere. Output a standalone illustration with genuinely transparent background, preserving the natural laptop shadow and only the subtle referenced plant/desk contact detail. Do not output a website screenshot.
```

## Остальные материалы

- Иконки: `lucide-react`, локальные SVG-компоненты, лицензия ISC.
- Шрифт Inter: `@fontsource/inter`, локальные файлы, SIL Open Font License.
- Знак Meet: простой SVG/компонент с иконкой камеры, без внешней загрузки.
- В приложении нет внешних шрифтов, трекеров и удалённых фотографий.
