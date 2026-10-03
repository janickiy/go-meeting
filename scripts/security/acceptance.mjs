// Выполняет реальную регрессию изолированных инфраструктурных образов без вывода secrets.
import { readFileSync } from "node:fs";
import { spawnSync } from "node:child_process";
import { resolve } from "node:path";

/** readConfig читает dotenv как данные, без исполнения shell-подстановок.
 * @args filename — абсолютный путь закрытой конфигурации тестового стенда.
 * @return словарь строковых параметров.
 */
function readConfig(filename) {
  const entries = readFileSync(filename, "utf8").split(/\r?\n/).flatMap((line) => {
    if (line.startsWith("#") || !line.includes("=")) return [];
    const offset = line.indexOf("=");
    return [[line.slice(0, offset), line.slice(offset + 1)]];
  });
  return Object.fromEntries(entries);
}

/** redact заменяет точные значения закрытых параметров в диагностическом выводе.
 * @args text — диагностический текст; config — конфигурация с тестовыми secrets.
 * @return текст без значений паролей, ключей и полного DSN.
 */
function redact(text, config) {
  for (const [key, value] of Object.entries(config)) {
    if (value.length >= 16 || /PASSWORD|SECRET|TOKEN|KEY/.test(key)) {
      if (value) text = text.split(value).join("[REDACTED]");
    }
  }
  return text;
}

const filename = process.argv[2];
if (!filename || resolve(filename) !== filename) throw new Error("Absolute isolated configuration required");
const config = readConfig(filename);
for (const key of ["POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DB", "REDIS_PASSWORD", "MINIO_ROOT_USER", "MINIO_ROOT_PASSWORD", "RABBIT_MQ_USER", "RABBIT_MQ_PASSWORD"]) {
  if (!config[key]) throw new Error(`Missing isolated parameter: ${key}`);
}
const encode = encodeURIComponent;
const environment = {
  ...process.env,
  RECORDER_STAGE1_TEST_POSTGRES_DSN: `postgres://${encode(config.POSTGRES_USER)}:${encode(config.POSTGRES_PASSWORD)}@127.0.0.1:25434/${encode(config.POSTGRES_DB)}?sslmode=disable`,
  RECORDER_STAGE2_TEST_REDIS_ADDR: "127.0.0.1:26381",
  RECORDER_STAGE2_TEST_REDIS_PASSWORD: config.REDIS_PASSWORD,
  RECORDER_STAGE4_TEST_MINIO_ENDPOINT: "127.0.0.1:29100",
  RECORDER_STAGE4_TEST_MINIO_ACCESS_KEY: config.MINIO_ROOT_USER,
  RECORDER_STAGE4_TEST_MINIO_SECRET_KEY: config.MINIO_ROOT_PASSWORD,
  RECORDER_STAGE4_TEST_RABBIT_URL: `amqp://${encode(config.RABBIT_MQ_USER)}:${encode(config.RABBIT_MQ_PASSWORD)}@127.0.0.1:25683/%2F`,
  RECORDER_STAGE4_RECORDING_E2E: "true",
  RECORDER_STAGE5_CHAT_E2E: "true",
  RECORDER_STAGE8_STORAGE_E2E: "true",
  RECORDER_TEST_FFMPEG: process.env.RECORDER_TEST_FFMPEG || "/opt/homebrew/bin/ffmpeg",
};
const result = spawnSync("go", ["test", "./tests/integration", "-run", "^(TestStageFourCompositeRecording|TestStageFiveChatFilesRead|TestStageEightRecordingStorage)$", "-v", "-count=1", "-timeout=5m"], {
  env: environment, encoding: "utf8", maxBuffer: 64 * 1024 * 1024,
});
process.stdout.write(redact(result.stdout || "", config));
process.stderr.write(redact(result.stderr || "", config));
if (result.error) process.stderr.write("Acceptance process could not complete\n");
process.exit(result.status ?? 1);
