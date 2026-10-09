// Собирает ресурсы макетов из уже установленных зависимостей проекта, без сети.
import { createRequire } from "node:module";
import {
  mkdir,
  copyFile,
  writeFile,
  readFile,
  readdir,
} from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";
const here = path.dirname(fileURLToPath(import.meta.url));
const project = path.resolve(here, "../..");
const require = createRequire(path.join(project, "frontend/package.json"));
const React = require("react");
const { renderToStaticMarkup } = require("react-dom/server");
const library = require("lucide-react");
const assets = path.join(here, "assets");
await mkdir(assets, { recursive: true });
const names = [
  "House",
  "Home",
  "Video",
  "VideoOff",
  "Mic",
  "MicOff",
  "Monitor",
  "MonitorUp",
  "ScreenShare",
  "MessageCircle",
  "MessagesSquare",
  "Users",
  "User",
  "UserRound",
  "UserPlus",
  "UserRoundPlus",
  "UserRoundCheck",
  "Calendar",
  "CalendarDays",
  "CalendarClock",
  "Clock",
  "Clock3",
  "CirclePlay",
  "Play",
  "Pause",
  "Square",
  "Circle",
  "Clapperboard",
  "Folder",
  "FolderPlus",
  "FolderInput",
  "FolderOpen",
  "Settings",
  "Settings2",
  "SlidersHorizontal",
  "Bell",
  "BellOff",
  "BellRing",
  "Search",
  "Plus",
  "X",
  "Check",
  "CheckCheck",
  "ChevronDown",
  "ChevronUp",
  "ChevronLeft",
  "ChevronRight",
  "ArrowLeft",
  "ArrowRight",
  "ArrowUpRight",
  "LogOut",
  "Link",
  "Link2",
  "Copy",
  "Ellipsis",
  "EllipsisVertical",
  "MoreHorizontal",
  "MoreVertical",
  "Menu",
  "Info",
  "CircleHelp",
  "CircleAlert",
  "AlertTriangle",
  "TriangleAlert",
  "CircleCheck",
  "CircleX",
  "Shield",
  "ShieldCheck",
  "Lock",
  "Mail",
  "Eye",
  "EyeOff",
  "Smile",
  "SmilePlus",
  "Paperclip",
  "Send",
  "Reply",
  "Pencil",
  "Trash2",
  "Download",
  "Upload",
  "File",
  "FileText",
  "Image",
  "Volume2",
  "VolumeX",
  "Headphones",
  "Laptop",
  "Smartphone",
  "Sun",
  "Moon",
  "Palette",
  "Type",
  "LoaderCircle",
  "RefreshCw",
  "Wifi",
  "WifiOff",
  "Unplug",
  "Maximize",
  "Minimize",
  "Captions",
  "Languages",
  "CheckCircle2",
  "ExternalLink",
  "CalendarCheck",
  "CalendarX",
  "CalendarPlus",
  "History",
  "BarChart3",
  "ChartNoAxesCombined",
  "ChartColumn",
  "List",
  "Grid2X2",
  "Grid3X3",
  "LayoutGrid",
  "LayoutList",
  "Hash",
  "Sparkles",
  "Star",
  "ShieldAlert",
  "CheckCircle",
  "CircleDot",
  "CircleStop",
  "ScreenShareOff",
  "MonitorOff",
  "Clipboard",
  "ChevronRight",
  "CircleUserRound",
  "FileImage",
  "FileVideo",
  "Music",
  "AudioLines",
  "ArrowDown",
  "FolderClosed",
  "Frown",
  "Inbox",
];
const icons = {};
for (const filename of await readdir(here)) {
  if (!filename.startsWith("screens-") || !filename.endsWith(".js")) continue;
  const source = await readFile(path.join(here, filename), "utf8");
  for (const match of source.matchAll(/['"]([A-Z][A-Za-z0-9]+)['"]/g))
    if (library[match[1]]) names.push(match[1]);
}
for (const name of new Set(names)) {
  if (!library[name]) continue;
  icons[name] = renderToStaticMarkup(
    React.createElement(library[name], {
      size: 20,
      strokeWidth: 1.75,
      "aria-hidden": true,
    }),
  );
}
await writeFile(
  path.join(assets, "icons.js"),
  `window.MeetrixIcons = ${JSON.stringify(icons)};\n`,
);
for (const script of ["cyrillic", "latin"])
  for (const weight of [400, 600]) {
    const file = `inter-${script}-${weight}-normal.woff2`;
    await copyFile(
      path.join(project, "frontend/node_modules/@fontsource/inter/files", file),
      path.join(assets, file),
    );
  }
for (const [source, name] of [
  ["brand-mark.svg", "brand-mark.svg"],
  ["media/auth-landscape.jpg", "auth-landscape.jpg"],
  ["media/meet-laptop.png", "meet-laptop.png"],
]) {
  await copyFile(
    path.join(project, "frontend/public", source),
    path.join(assets, name),
  );
}
const audits = {};
await mkdir(path.join(here, "audit"), { recursive: true });
for (const name of ["meetings", "messaging", "recordings"]) {
  const source = path.join(
    project,
    "tmp",
    `design-audit-${name}-20261009.json`,
  );
  const raw = await readFile(source, "utf8");
  audits[name] = JSON.parse(raw);
  await copyFile(source, path.join(here, "audit", `${name}.json`));
}
await writeFile(
  path.join(here, "audit-data.js"),
  `window.MeetrixSourceAudits = ${JSON.stringify(audits)};\n`,
);
console.log(
  `Ресурсы готовы: ${Object.keys(icons).length} иконок, 4 шрифта, 3 изображения.`,
);
