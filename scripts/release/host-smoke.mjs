// Создаёт отдельный синтетический аккаунт, затем запускает неизменяемый smoke.
// Пароль существует только в памяти процесса и не попадает в аргументы или отчёт.
import { randomUUID } from 'node:crypto';
import { spawnSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
const [binary, manifestFile, environment] = process.argv.slice(2);
if (!['staging','production'].includes(environment) || !binary?.startsWith('/') || !manifestFile?.startsWith('/')) throw new Error('Explicit reviewed target required');
const manifest = JSON.parse(readFileSync(manifestFile,'utf8'));
const email = `release-${randomUUID()}@example.test`, password = randomUUID();
const response = await fetch('https://meeting.janickiy.com/api/v1/auth/register',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({email,password,displayName:'Проверка публикации'})});
if (!response.ok) throw new Error(`Synthetic registration: HTTP ${response.status}`);
const options = ['--environment',environment,'--base-url','https://meeting.janickiy.com','--version',manifest.version,'--commit',manifest.commit];
if (environment === 'staging') options.push('--exercise');
const result = spawnSync(binary,options,{env:{...process.env,SMOKE_EMAIL:email,SMOKE_PASSWORD:password},encoding:'utf8',timeout:150_000});
process.stdout.write(result.stdout||''); process.stderr.write(result.stderr||'');
process.exit(result.status ?? 1);
