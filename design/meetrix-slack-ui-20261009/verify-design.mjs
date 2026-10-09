// Проверяет автономную галерею; рабочее приложение и сеть не используются.
import { readFile, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";
import vm from "node:vm";
const root = path.dirname(fileURLToPath(import.meta.url));
const files = [
  "assets/icons.js",
  "runtime.js",
  "screens-meetings.js",
  "screens-messaging.js",
  "screens-recordings.js",
  "screens-conference.js",
  "screens-kit.js",
];
const context = vm.createContext({
  window: {},
  URLSearchParams,
  console,
  location: { search: "" },
  document: { documentElement: { dataset: { theme: "light" } } },
});
for (const file of files)
  vm.runInContext(await readFile(path.join(root, file), "utf8"), context, {
    filename: file,
  });
const screens = context.window.MeetrixDesign.screens;
const errors = [];
let renders = 0;
let links = 0;
for (const screen of screens) {
  for (const key of [
    "id",
    "title",
    "group",
    "route",
    "permissions",
    "sources",
    "responsive",
  ])
    if (!screen[key]) errors.push(`${screen.id}: отсутствует ${key}`);
  for (const state of screen.states || [{ id: "default" }]) {
    for (const width of [360, 390, 768, 834, 1024, 1280, 1440, 1920]) {
      try {
        const html = screen.render({
          state: state.id,
          width,
          theme: "light",
          scale: 100,
          role: screen.id === "admin" ? "admin" : "owner",
        });
        if (typeof html !== "string" || !html.trim())
          throw Error("Пустая разметка");
        if (/undefined|\[object Object\]/.test(html))
          throw Error("Некорректное значение в разметке");
        for (const match of html.matchAll(/data-go="([^"]+)"/g)) {
          links++;
          if (!screens.some((x) => x.id === match[1]))
            throw Error(`Несуществующий экран ${match[1]}`);
        }
        renders++;
      } catch (error) {
        errors.push(`${screen.id}/${state.id}/${width}: ${error.message}`);
      }
    }
  }
}
const manifest = screens.map(
  ({ render, afterRender, ...metadata }) => metadata,
);
await writeFile(
  path.join(root, "screen-manifest.json"),
  JSON.stringify(manifest, null, 2) + "\n",
);
const report = {
  variant: '02 — Командная среда, Slack-inspired',
  date: "2026-10-09",
  scope:
    "Статическая проверка самостоятельного дизайн-прототипа, не production-приложения",
  screens: screens.length,
  states: screens.reduce((n, s) => n + (s.states?.length || 1), 0),
  renderChecks: renders,
  navigationReferences: links,
  viewportInputs: [360, 390, 768, 834, 1024, 1280, 1440, 1920],
  note: "Рендер проверяется на строках HTML. Фактическую CSS-адаптацию подтверждает отдельная визуальная проверка в браузере.",
  errors,
};
try {
  report.browserVerification = JSON.parse(await readFile(path.join(root, 'browser-qa.json'), 'utf8')).summary;
  report.browserReport = 'browser-qa.json';
} catch (error) {
  if (error.code !== 'ENOENT') throw error;
}
await writeFile(
  path.join(root, "qa-report.json"),
  JSON.stringify(report, null, 2) + "\n",
);
console.log(JSON.stringify(report, null, 2));
if (errors.length) process.exitCode = 1;
