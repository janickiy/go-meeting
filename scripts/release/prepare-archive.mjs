// Формирует новый неизменяемый delivery-пакет из уже проверенных байтов образов.
// OCI manifest digest отличается от config digest, который указывает Trivy.
import { createHash } from 'node:crypto';
import { readFileSync, writeFileSync, existsSync, mkdirSync, copyFileSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { resolve, dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const [input, output, infrastructureFile] = process.argv.slice(2);
if (!input || !output || resolve(input) !== input || resolve(output) !== output || existsSync(output)) throw new Error('New absolute output directory required');
const root = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const sha = bytes => createHash('sha256').update(bytes).digest('hex');
const fileSha = filename => execFileSync('openssl', ['dgst', '-sha256', filename], {encoding:'utf8'}).trim().split(/\s+/).at(-1);
const manifest = JSON.parse(readFileSync(join(input, 'release.json'), 'utf8'));
if (manifest.artifactMode !== 'archive' || !manifest.securityScanned || manifest.sourceDirty || !manifest.sourceFingerprintVerified) throw new Error('Scanned clean source release required');
const archive = join(input, 'images.tar');
if (fileSha(archive) !== manifest.archiveSHA256) throw new Error('Archive checksum mismatch');
const member = name => execFileSync('tar', ['-xOf', archive, name], {maxBuffer:4*1024*1024});
const index = JSON.parse(member('index.json'));
// Повторно используемые immutable образы могут быть сохранены без RepoTags.
// Связываем их с отчётом по проверенным байтам manifest/config, а не по имени.
const saved = index.manifests.map(item => {
  const digest = item.digest;
  if (!/^sha256:[a-f0-9]{64}$/.test(digest)) throw new Error('Invalid OCI digest');
  const bytes = member(`blobs/sha256/${digest.slice(7)}`);
  if (`sha256:${sha(bytes)}` !== digest) throw new Error('OCI manifest checksum mismatch');
  const configID = JSON.parse(bytes).config?.digest;
  if (!/^sha256:[a-f0-9]{64}$/.test(configID)) throw new Error('Invalid configuration digest');
  return {digest, configID};
});
const originalIDs = {...manifest.images};
if (manifest.infrastructureReused) {
  if (!infrastructureFile || resolve(infrastructureFile) !== infrastructureFile || fileSha(infrastructureFile) !== manifest.infrastructureManifestSHA256) throw new Error('Checksummed infrastructure manifest required');
  const infrastructure = JSON.parse(readFileSync(infrastructureFile, 'utf8'));
  if (infrastructure.version !== manifest.infrastructureVersion || infrastructure.sourceDirty || !infrastructure.securityScanned || !infrastructure.sourceFingerprintVerified || !/^[a-f0-9]{40}$/.test(infrastructure.commit) || !/^[a-f0-9]{64}$/.test(infrastructure.sourceSHA256)) throw new Error('Invalid infrastructure provenance');
  for (const service of ['minio','postgres','redis','rabbitmq','coturn','proxy','prometheus']) {
    if (infrastructure.images[service] !== originalIDs[service]) throw new Error(`Reused infrastructure changed: ${service}`);
  }
  manifest.infrastructureSource = {version:infrastructure.version,commit:infrastructure.commit,sourceSHA256:infrastructure.sourceSHA256};
}
manifest.imageConfigIDs = {};
for (const service of Object.keys(manifest.images)) {
  const scanName = `${service}.scan.json`, sbomName = `${service}.sbom.json`;
  const scanBytes = readFileSync(join(input, scanName));
  const sbomBytes = readFileSync(join(input, sbomName));
  const evidence = manifest.securityEvidence[service];
  if (sha(scanBytes) !== evidence.scanSHA256 || sha(sbomBytes) !== evidence.sbomSHA256) throw new Error(`Evidence checksum mismatch: ${service}`);
  const scan = JSON.parse(scanBytes);
  if (!Array.isArray(scan.Results) || scan.Results.some(r => r.Vulnerabilities?.some(v => ['HIGH','CRITICAL'].includes(v.Severity)))) throw new Error(`Security gate failed: ${service}`);
  if (scan.Metadata.ImageConfig.architecture !== manifest.platform.split('/')[1]) throw new Error(`Architecture mismatch: ${service}`);
  const matches = saved.filter(item => item.configID === scan.Metadata.ImageID);
  if (matches.length !== 1) throw new Error(`Ambiguous saved image: ${service}`);
  const {digest, configID} = matches[0];
  if (`sha256:${sha(member(`blobs/sha256/${configID.slice(7)}`))}` !== configID) throw new Error('Configuration checksum mismatch');
  manifest.images[service] = digest;
  manifest.imageConfigIDs[service] = configID;
}
mkdirSync(output, {mode:0o700});
copyFileSync(archive, join(output,'images.tar'));
for (const service of Object.keys(manifest.images)) for (const suffix of ['scan','sbom']) copyFileSync(join(input,`${service}.${suffix}.json`),join(output,`${service}.${suffix}.json`));
// Образы связаны с исходным build snapshot. Инструменты развёртывания имеют
// отдельную проверяемую контрольную сумму: подмена происхождения образов запрещена.
const files = ['docker-compose.production.yml','docker-compose.hardened.yml','docker-compose.shared-host.yml','docker-compose.release-local.yml',
  'docker-compose.release-local-grafana.yml','docker-compose.release-local-hardened.yml','dockers/production/nginx.conf',
  ...['common.sh','release.sh','backup.sh','restore-check.sh','smoke.sh'].map(n => `scripts/release/${n}`),
  'dockers/production/nginx-external-tls.conf','dockers/production/routes.conf','dockers/turn/entrypoint.sh','dockers/turn/turnserver.conf',
  'dockers/observability/prometheus.yml','dockers/observability/alerts.yml'];
const checksums = [];
for (const name of files.sort()) {
  const dest = join(output,'deployment',name);
  mkdirSync(dirname(dest),{recursive:true,mode:0o700});
  copyFileSync(join(root,name),dest);
  checksums.push(`${fileSha(dest)}  deployment/${name}`);
}
const tooling = `${checksums.join('\n')}\n`;
writeFileSync(join(output,'DEPLOYMENT_SHA256SUMS'),tooling,{mode:0o600});
manifest.deploymentSHA256 = sha(tooling);
manifest.originalBuildImageIDs = originalIDs;
manifest.archiveBinding = 'OCI manifest -> config digest -> Trivy report';
writeFileSync(join(output,'release.json'),`${JSON.stringify(manifest,null,2)}\n`,{mode:0o600});
writeFileSync(join(output,'SHA256SUMS'),`${fileSha(join(output,'release.json'))}  release.json\n${manifest.archiveSHA256}  images.tar\n`,{mode:0o600});
process.stdout.write('Delivery package bound to exported OCI images and checksummed deployment tooling.\n');
